package compaction

import "testing"

func TestMarkerRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, ok, err := Load("s1"); err != nil || ok {
		t.Fatalf("no marker expected: ok=%v err=%v", ok, err)
	}

	if err := Save("s1", Marker{Started: "2026-08-07T10:00:00Z", Trigger: "auto"}); err != nil {
		t.Fatal(err)
	}
	m, ok, err := Load("s1")
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if m.Trigger != "auto" {
		t.Errorf("trigger = %q, want auto", m.Trigger)
	}
	if ts, ok := m.StartedAt(); !ok || ts.IsZero() {
		t.Errorf("StartedAt parse failed: %v ok=%v", ts, ok)
	}

	if err := Remove("s1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Load("s1"); ok {
		t.Error("marker should be gone after Remove")
	}
	if err := Remove("s1"); err != nil {
		t.Errorf("Remove of a missing marker must not error: %v", err)
	}
}

func TestRejectsTraversal(t *testing.T) {
	for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
		if err := Save(id, Marker{}); err == nil {
			t.Errorf("Save(%q) should reject an unsafe id", id)
		}
	}
}
