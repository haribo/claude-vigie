package store

import (
	"context"
	"testing"
	"time"
)

func TestAcquireLease(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	ttl := 12 * time.Minute

	ok, exp, err := st.AcquireLease(ctx, "A", ttl, now)
	if err != nil || !ok {
		t.Fatalf("A acquire (free): ok=%v err=%v", ok, err)
	}
	if exp == "" {
		t.Error("empty expiry")
	}

	ok, _, err = st.AcquireLease(ctx, "B", ttl, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("B should be denied while A holds a valid lease")
	}

	ok, _, err = st.AcquireLease(ctx, "A", ttl, now.Add(2*time.Minute))
	if err != nil || !ok {
		t.Errorf("A renew: ok=%v err=%v", ok, err)
	}

	ok, _, err = st.AcquireLease(ctx, "B", ttl, now.Add(30*time.Minute))
	if err != nil || !ok {
		t.Errorf("B after expiry: ok=%v err=%v", ok, err)
	}
}
