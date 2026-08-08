package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentWritesDoNotBusyError guards #372: the watcher re-reports every
// session (plus usage) every ~2 s, so the daemon runs many concurrent writes.
// busy_timeout must apply to every pooled connection — not just the first — or
// the extra connections fail immediately with SQLITE_BUSY, surfacing as
// intermittent 500s. With the pragma carried in the DSN, contending writers
// wait instead of erroring, so none of these writes fails.
func TestConcurrentWritesDoNotBusyError(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const writers, each = 16, 40
	var wg sync.WaitGroup
	errs := make(chan error, writers*each)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				sess := Session{ID: fmt.Sprintf("s-%d-%d", w, i), Machine: "m", Status: "idle"}
				if err := st.UpsertSession(context.Background(), sess); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	n := 0
	var first error
	for err := range errs {
		if first == nil {
			first = err
		}
		n++
	}
	if n > 0 {
		t.Fatalf("%d/%d concurrent writes failed, first: %v", n, writers*each, first)
	}
}

// TestPragmasApplyToEveryConnection guards #372 directly: busy_timeout and
// journal_mode must be set on any connection the pool hands out, not only the
// one Open happened to touch. Opening several connections at once and reading
// their pragmas back proves the DSN applies them per connection.
func TestPragmasApplyToEveryConnection(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "pragmas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// Force the pool to open several distinct connections simultaneously by
	// holding open transactions, then check each one's pragmas.
	const conns = 4
	ctx := context.Background()
	var txs []*sql.Tx
	for i := 0; i < conns; i++ {
		tx, err := st.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		txs = append(txs, tx)
	}
	t.Cleanup(func() {
		for _, tx := range txs {
			_ = tx.Rollback()
		}
	})
	for i, tx := range txs {
		var busy int
		if err := tx.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatalf("conn %d: reading busy_timeout: %v", i, err)
		}
		if busy != 5000 {
			t.Errorf("conn %d: busy_timeout = %d, want 5000 (pragma missing on this connection)", i, busy)
		}
		var mode string
		if err := tx.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatalf("conn %d: reading journal_mode: %v", i, err)
		}
		if mode != "wal" {
			t.Errorf("conn %d: journal_mode = %q, want wal", i, mode)
		}
	}
}
