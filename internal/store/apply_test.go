package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// #512. The server read a session, merged the report in Go, and wrote it back,
// with nothing holding the row in between. Measured on two concurrent cycles, the
// overlap is ~300µs wide: both reads land before either write, so the second
// write is computed from a state the first has already replaced.
//
// Racing for it is not a test — the outcome depends on which side happens to
// commit last, and it committed in the "right" order 200 times out of 200 on the
// machine this was found on. What is testable is the *mechanism*: a second cycle
// must observe the first one's write, deterministically.

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "apply.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// The guarantee, stated so it cannot pass by luck: the second cycle is held until
// the first has committed, so it merges against what the first wrote.
func TestASecondApplySeesTheFirstOnesWrite(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()

	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := st.ApplySession(ctx, "s", func(current Session, _ bool) Session {
			close(started)
			// Hold the cycle open. Without the lock the other writer reads now,
			// sees an empty row, and overwrites what this one is about to commit.
			time.Sleep(50 * time.Millisecond)
			current.ID, current.Machine, current.Status = "s", "m", "waiting"
			return current
		})
		if err != nil {
			t.Error(err)
		}
	}()

	<-started
	time.Sleep(5 * time.Millisecond) // firmly inside the first cycle

	var saw string
	_, err := st.ApplySession(ctx, "s", func(current Session, isNew bool) Session {
		saw = current.Status
		if isNew {
			saw = "<absent>"
		}
		current.ID, current.Machine, current.Status = "s", "m", "idle"
		return current
	})
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	if saw != "waiting" {
		t.Errorf("the second cycle merged against %q — it ran inside the first one, and the first one's write is lost", saw)
	}
}

// And the whole point of holding it: no update is dropped. Each writer adds to a
// running total, which only survives if every cycle reads what the previous one
// committed.
func TestConcurrentAppliesLoseNoUpdate(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()

	const writers = 16
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.ApplySession(ctx, "s", func(current Session, _ bool) Session {
				current.ID, current.Machine = "s", "m"
				current.Usage.OutputTokens += 10 // a read-modify-write, on purpose
				return current
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	got, err := st.GetSession(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(writers * 10); got.Usage.OutputTokens != want {
		t.Errorf("output tokens = %d, want %d — %d update(s) were computed from a stale row",
			got.Usage.OutputTokens, want, (want-got.Usage.OutputTokens)/10)
	}
}

// A cycle on a session that does not exist yet must say so, or the caller's merge
// cannot tell a first report from an update.
func TestApplyReportsWhetherTheSessionIsNew(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()

	var first, second bool
	if _, err := st.ApplySession(ctx, "s", func(c Session, isNew bool) Session {
		first = isNew
		c.ID, c.Machine, c.Status = "s", "m", "idle"
		return c
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplySession(ctx, "s", func(c Session, isNew bool) Session {
		second = isNew
		return c
	}); err != nil {
		t.Fatal(err)
	}
	if !first || second {
		t.Errorf("isNew = %v then %v, want true then false", first, second)
	}
}
