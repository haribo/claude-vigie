package presence

import (
	"os"
	"testing"
)

func TestSaveLoadDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, ok, err := Load("sess-1"); err != nil || ok {
		t.Fatalf("Load before save: ok=%v err=%v, want ok=false nil", ok, err)
	}

	want := Mapping{PID: 4242, StartTime: 99999}
	if err := Save("sess-1", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := Load("sess-1")
	if err != nil || !ok {
		t.Fatalf("Load after save: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}

	if err := Delete("sess-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := Load("sess-1"); ok {
		t.Error("Load after delete: ok=true, want false")
	}
	if err := Delete("sess-1"); err != nil {
		t.Errorf("Delete when absent: %v, want nil", err)
	}
}

func TestPathForRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "../x"} {
		if _, err := pathFor(bad); err == nil {
			t.Errorf("pathFor(%q) = nil error, want rejection", bad)
		}
	}
}

func TestReadStatAndAlive(t *testing.T) {
	self := os.Getpid()
	comm, ppid, start, err := readStat(self)
	if err != nil {
		t.Fatalf("readStat(self): %v", err)
	}
	if comm == "" || ppid <= 0 || start == 0 {
		t.Fatalf("readStat(self) = comm=%q ppid=%d start=%d, want all set", comm, ppid, start)
	}

	if !Alive(Mapping{PID: self, StartTime: start}) {
		t.Error("Alive(self with correct start_time) = false, want true")
	}
	// A mismatched start time simulates a reused pid → must read as not alive.
	if Alive(Mapping{PID: self, StartTime: start + 1}) {
		t.Error("Alive(self with wrong start_time) = true, want false (pid-reuse guard)")
	}
	// A pid that almost certainly does not exist.
	if Alive(Mapping{PID: 2 << 30, StartTime: 1}) {
		t.Error("Alive(nonexistent pid) = true, want false")
	}
}
