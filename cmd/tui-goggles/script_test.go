package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/your-username/tui-goggles/internal/script"
	"github.com/your-username/tui-goggles/internal/terminal"
)

// runTestScript runs src against `cat` (which echoes typed input) and
// returns the exit code and the JSON result.
func runTestScript(t *testing.T, src string) (int, ScriptResult) {
	t.Helper()
	steps, err := script.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	cfg := config{
		cols: 40, rows: 5,
		stableTimeout: 2 * time.Second, stableTime: 50 * time.Millisecond,
		inputDelay: 10 * time.Millisecond, outputFormat: "json", outputFile: out,
		timeout: 30 * time.Second, trim: true,
	}
	term, err := terminal.New("cat", nil, terminal.Options{Cols: 40, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	var timedOut atomic.Bool
	code := runScript(term, steps, "cat", nil, cfg, nil, &TimingInfo{}, time.Now(), &timedOut)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var res ScriptResult
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	return code, res
}

func TestRunScriptSuccess(t *testing.T) {
	code, res := runTestScript(t, `
type hello world
wait-for hello world
capture typed
assert hello
assert-not goodbye
key enter type:"second line"
capture
resize 30x4
`)
	if code != ExitSuccess || res.Failed != nil {
		t.Fatalf("exit %d, failed %+v", code, res.Failed)
	}
	if res.StepsRun != 8 {
		t.Errorf("steps_run = %d; want 8", res.StepsRun)
	}
	if len(res.Captures) != 2 {
		t.Fatalf("got %d captures; want 2", len(res.Captures))
	}
	if c := res.Captures[0]; c.Name != "typed" || c.Line != 4 || !strings.Contains(c.Screen, "hello world") {
		t.Errorf("first capture = %+v", c)
	}
	if c := res.Captures[1]; c.Name != "capture 2" || !strings.Contains(c.Screen, "second line") {
		t.Errorf("second capture = %+v", c)
	}
	if res.Process == nil || res.Process.EndedBy == "" {
		t.Errorf("process = %+v", res.Process)
	}
}

func TestRunScriptFinalCaptureWhenNoneRequested(t *testing.T) {
	code, res := runTestScript(t, "type abc\n")
	if code != ExitSuccess || len(res.Captures) != 1 || res.Captures[0].Name != "final" {
		t.Fatalf("exit %d, captures %+v", code, res.Captures)
	}
}

func TestRunScriptAssertFailure(t *testing.T) {
	code, res := runTestScript(t, "type abc\ncapture before\nassert xyz\ncapture after\n")
	if code != ExitAssertionFailed {
		t.Errorf("exit %d; want %d", code, ExitAssertionFailed)
	}
	if res.Failed == nil || res.Failed.Line != 3 || res.Failed.Step != "assert xyz" {
		t.Errorf("failed = %+v", res.Failed)
	}
	if res.StepsRun != 2 {
		t.Errorf("steps_run = %d; want 2", res.StepsRun)
	}
	names := []string{}
	for _, c := range res.Captures {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "before,failure" {
		t.Errorf("captures = %v; want before,failure", names)
	}
}

func TestRunScriptAssertNotFailure(t *testing.T) {
	code, res := runTestScript(t, "type abc\nassert-not abc\n")
	if code != ExitAssertionFailed || res.Failed == nil || res.Failed.Line != 2 {
		t.Errorf("exit %d, failed %+v", code, res.Failed)
	}
}

func TestRunScriptWaitTimeout(t *testing.T) {
	code, res := runTestScript(t, "wait-for never\n")
	if code != ExitTimeout || res.Failed == nil || !strings.Contains(res.Failed.Error, "did not appear") {
		t.Errorf("exit %d, failed %+v", code, res.Failed)
	}
}
