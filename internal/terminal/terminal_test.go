package terminal

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestResize(t *testing.T) {
	// The shell reports its size, waits for a line, then reports it again.
	term, err := New("sh", []string{"-c", `stty size; read x; stty size; sleep 5`}, Options{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()

	if err := term.WaitForText("24 80", 5*time.Second); err != nil {
		t.Fatalf("initial size: %v\n%s", err, term.Screenshot())
	}
	if err := term.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if cols, rows := term.Size(); cols != 100 || rows != 30 {
		t.Errorf("Size() = %dx%d; want 100x30", cols, rows)
	}
	if err := term.SendKeys("\r"); err != nil {
		t.Fatal(err)
	}
	if err := term.WaitForText("30 100", 5*time.Second); err != nil {
		t.Fatalf("size after resize: %v\n%s", err, term.Screenshot())
	}

	screen := term.Screenshot()
	lines := strings.Split(strings.TrimSuffix(screen, "\n"), "\n")
	if len(lines) != 30 || len([]rune(lines[0])) != 100 {
		t.Errorf("screen is %d lines of %d cols; want 30 of 100", len(lines), len([]rune(lines[0])))
	}
	// Content from before the resize is kept.
	if !strings.Contains(screen, "24 80") {
		t.Errorf("pre-resize output lost:\n%s", screen)
	}
}

func TestResizeRejectsInvalidSize(t *testing.T) {
	term, err := New("sh", []string{"-c", "sleep 5"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	for _, size := range [][2]int{{0, 10}, {10, 0}, {-1, 5}} {
		if err := term.Resize(size[0], size[1]); err == nil {
			t.Errorf("Resize(%d, %d) succeeded; want error", size[0], size[1])
		}
	}
	if cols, rows := term.Size(); cols != 80 || rows != 24 {
		t.Errorf("Size() = %dx%d after rejected resizes; want 80x24", cols, rows)
	}
}

func TestCloseSendsHangupFirst(t *testing.T) {
	marker := t.TempDir() + "/saved"
	script := `trap 'echo bye > ` + marker + `; exit 0' HUP; echo ready; while :; do sleep 0.05; done`
	term, err := New("sh", []string{"-c", script}, Options{Grace: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := term.WaitForText("ready", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	term.Close()

	st := term.ExitStatus()
	if st.EndedBy != EndedHangup || st.ExitCode != 0 {
		t.Errorf("ExitStatus() = %+v; want ended by hangup with code 0", st)
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "bye" {
		t.Errorf("HUP handler did not run: %q, %v", data, err)
	}
}

func TestCloseKillsAfterGrace(t *testing.T) {
	term, err := New("sh", []string{"-c", `trap '' HUP; echo ready; while :; do sleep 0.05; done`}, Options{Grace: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := term.WaitForText("ready", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	term.Close()
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond || elapsed > 3*time.Second {
		t.Errorf("Close took %v; want about the 200ms grace period", elapsed)
	}
	st := term.ExitStatus()
	if st.EndedBy != EndedKilled || st.Signal != "SIGKILL" || st.ExitCode != -1 {
		t.Errorf("ExitStatus() = %+v; want killed by SIGKILL", st)
	}
}

func TestCloseWithoutGraceKillsImmediately(t *testing.T) {
	term, err := New("sh", []string{"-c", `echo ready; sleep 10`}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := term.WaitForText("ready", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	term.Close()
	term.Close() // safe to call twice
	if st := term.ExitStatus(); st.EndedBy != EndedKilled {
		t.Errorf("ExitStatus() = %+v; want killed", st)
	}
}

func TestExitStatusWhenProcessExitsOnItsOwn(t *testing.T) {
	term, err := New("sh", []string{"-c", `exit 3`}, Options{Grace: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := term.Wait(); err == nil {
		t.Error("Wait() = nil; want exit error")
	}
	if !term.Exited() || term.IsRunning() {
		t.Error("process should be reported as exited")
	}
	term.Close()
	if st := term.ExitStatus(); st.EndedBy != EndedExited || st.ExitCode != 3 {
		t.Errorf("ExitStatus() = %+v; want exited with code 3", st)
	}
}
