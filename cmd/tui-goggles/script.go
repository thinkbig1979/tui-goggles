package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/your-username/tui-goggles/internal/script"
	"github.com/your-username/tui-goggles/internal/terminal"
)

// ScriptResult is the output of a -script run.
type ScriptResult struct {
	Captures []CaptureResult      `json:"captures"`
	Command  string               `json:"command"`
	StepsRun int                  `json:"steps_run"`
	Failed   *StepFailure         `json:"failed,omitempty"`
	Checks   map[string]bool      `json:"checks,omitempty"`
	Timing   *TimingInfo          `json:"timing,omitempty"`
	Process  *terminal.ExitStatus `json:"process,omitempty"`
}

// StepFailure describes the step that stopped a script.
type StepFailure struct {
	Line  int    `json:"line"`
	Step  string `json:"step"`
	Error string `json:"error"`
}

// loadScript reads and parses the -script file ("-" for stdin).
func loadScript(path string) ([]script.Step, error) {
	f := os.Stdin
	if path != "-" {
		var err error
		if f, err = os.Open(path); err != nil {
			return nil, err
		}
		defer f.Close()
	}
	steps, err := script.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	return steps, nil
}

// scriptRunner runs script steps against one terminal session.
type scriptRunner struct {
	term     *terminal.Terminal
	command  string
	args     []string
	cfg      config
	timedOut *atomic.Bool

	result   ScriptResult
	captures int
}

// runScript runs steps, closes the app, writes output and returns the exit code.
func runScript(term *terminal.Terminal, steps []script.Step, command string, args []string,
	cfg config, timing *TimingInfo, startTime time.Time, timedOut *atomic.Bool) int {
	r := &scriptRunner{term: term, command: command, args: args, cfg: cfg, timedOut: timedOut}
	r.result.Command = strings.TrimSpace(command + " " + strings.Join(args, " "))

	exitCode := ExitSuccess
	keysStart := time.Now()
	for _, st := range steps {
		code, err := r.runStep(st)
		if err == nil && timedOut.Load() {
			code, err = ExitTimeout, fmt.Errorf("overall -timeout %v exceeded", cfg.timeout)
		}
		if err != nil {
			r.result.Failed = &StepFailure{Line: st.Line, Step: st.Text, Error: err.Error()}
			fmt.Fprintf(os.Stderr, "Step failed at line %d (%s): %v\n", st.Line, st.Text, err)
			r.capture("failure", st.Line)
			exitCode = code
			break
		}
		r.result.StepsRun++
	}
	timing.KeysMs = time.Since(keysStart).Milliseconds()

	// Without explicit captures, record the final screen.
	if r.captures == 0 && r.result.Failed == nil {
		_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)
		r.capture("final", 0)
	}

	// -check and -assert flags apply to the screen at the end of the script.
	final := r.result.Captures[len(r.result.Captures)-1].Screen
	if len(cfg.checks) > 0 {
		r.result.Checks = make(map[string]bool)
		for _, c := range cfg.checks {
			r.result.Checks[c] = strings.Contains(final, c)
		}
	}
	if exitCode == ExitSuccess {
		for _, a := range cfg.asserts {
			if !strings.Contains(final, a) {
				fmt.Fprintf(os.Stderr, "Assertion failed: text %q not found on screen\n", a)
				exitCode = ExitAssertionFailed
				break
			}
		}
	}

	timing.TotalMs = time.Since(startTime).Milliseconds()
	r.result.Timing = timing

	exitedOnItsOwn := term.Exited()
	term.Close()
	status := term.ExitStatus()
	r.result.Process = &status

	if !cfg.quiet {
		writeOutput(formatScriptResult(r.result, cfg), cfg)
	}
	if exitCode == ExitSuccess && exitedOnItsOwn && status.ExitCode != 0 {
		fmt.Fprintf(os.Stderr, "Error: command exited with status %d before the script finished\n", status.ExitCode)
		return ExitCommandError
	}
	return exitCode
}

// runStep runs one step. On failure it returns the exit code and an error.
func (r *scriptRunner) runStep(st script.Step) (int, error) {
	term, cfg := r.term, r.cfg
	switch st.Kind {
	case script.Input:
		for _, a := range st.Actions {
			if err := sendAction(term, a); err != nil {
				return ExitGeneralError, fmt.Errorf("sending %q: %w", a.Token, err)
			}
			time.Sleep(cfg.inputDelay)
		}
		if cfg.captureEach {
			_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)
			r.capture(st.Text, st.Line)
			r.captures++
		}

	case script.WaitFor:
		if err := term.WaitForText(st.Arg, cfg.stableTimeout); err != nil {
			return ExitTimeout, fmt.Errorf("text %q did not appear within %v", st.Arg, cfg.stableTimeout)
		}

	case script.WaitGone:
		if err := term.WaitForTextGone(st.Arg, cfg.stableTimeout); err != nil {
			return ExitTimeout, fmt.Errorf("text %q was still on screen after %v", st.Arg, cfg.stableTimeout)
		}

	case script.WaitStable:
		_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)

	case script.Sleep:
		time.Sleep(st.Duration)

	case script.Capture:
		_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)
		name := st.Arg
		if name == "" {
			name = fmt.Sprintf("capture %d", r.captures+1)
		}
		r.capture(name, st.Line)
		r.captures++

	case script.Assert, script.AssertNot:
		_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)
		found := strings.Contains(term.Screenshot(), st.Arg)
		if st.Kind == script.Assert && !found {
			return ExitAssertionFailed, fmt.Errorf("text %q not found on screen", st.Arg)
		}
		if st.Kind == script.AssertNot && found {
			return ExitAssertionFailed, fmt.Errorf("text %q is on screen", st.Arg)
		}

	case script.AssertStyle:
		return ExitGeneralError, fmt.Errorf("assert-style is not supported yet")
	}
	return ExitSuccess, nil
}

// capture records the current screen under name.
func (r *scriptRunner) capture(name string, line int) {
	c := captureScreen(r.term, r.command, r.args, r.cfg, nil)
	c.Name = name
	c.Line = line
	r.result.Captures = append(r.result.Captures, c)
}

func formatScriptResult(res ScriptResult, cfg config) string {
	if cfg.outputFormat == "json" {
		return formatAnyJSON(res)
	}
	var sb strings.Builder
	for i, c := range res.Captures {
		if i > 0 {
			sb.WriteString("\n")
		}
		if c.Line > 0 {
			fmt.Fprintf(&sb, "--- %s (line %d) ---\n", c.Name, c.Line)
		} else {
			fmt.Fprintf(&sb, "--- %s ---\n", c.Name)
		}
		sb.WriteString(c.Screen)
	}
	return sb.String()
}
