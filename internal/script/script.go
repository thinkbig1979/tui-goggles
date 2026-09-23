// Package script parses tui-goggles step scripts: one step per line,
// run in order against a single running application.
//
//	# comment
//	key down down enter        # one or more -keys tokens
//	type hello world           # rest of the line, verbatim
//	paste line one\nline two   # rest of the line as a bracketed paste
//	click 12,0                 # mouse verbs, with optional modifiers
//	shift+click 12,0 right
//	drag 5,3-20,3
//	wheel-down 10,5
//	resize 100x30
//	wait-for Saved             # wait until text appears
//	wait-gone Loading          # wait until text disappears
//	wait-stable                # wait until the screen stops changing
//	sleep 200ms
//	capture after-save         # record the screen under a name
//	assert Saved               # fail (exit 3) unless the text is on screen
//	assert-not Error           # fail unless the text is absent
//	assert-style text="Tab 2" reverse bold
//
// Text arguments are taken verbatim to the end of the line; one pair of
// surrounding quotes is removed (to keep leading or trailing spaces) and
// backslash escapes are processed as in -keys.
package script

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/your-username/tui-goggles/internal/input"
)

// Kind identifies a step type.
type Kind int

const (
	// Input sends Actions to the application.
	Input Kind = iota
	WaitFor
	WaitGone
	WaitStable
	Sleep
	Capture
	Assert
	AssertNot
	AssertStyle
)

// Step is one parsed script line.
type Step struct {
	Line     int    // 1-based line number
	Text     string // the line as written, trimmed
	Kind     Kind
	Actions  []input.Action // Input
	Arg      string         // text for waits, asserts, capture names and style specs
	Duration time.Duration  // Sleep
}

// Parse reads a script.
func Parse(r io.Reader) ([]Step, error) {
	var steps []Step
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		step, err := parseLine(trimmed)
		if err != nil {
			return nil, fmt.Errorf("line %d: %q: %w", line, trimmed, err)
		}
		step.Line = line
		step.Text = trimmed
		steps = append(steps, step)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return steps, nil
}

func parseLine(line string) (Step, error) {
	verb, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimLeft(rest, " \t")
	lverb := strings.ToLower(verb)

	switch lverb {
	case "key", "keys":
		tokens, err := input.Tokenize(rest)
		if err != nil {
			return Step{}, err
		}
		if len(tokens) == 0 {
			return Step{}, fmt.Errorf("%s needs at least one key", verb)
		}
		var actions []input.Action
		for _, tok := range tokens {
			a, err := input.ParseToken(tok)
			if err != nil {
				return Step{}, fmt.Errorf("key %q: %w", tok, err)
			}
			actions = append(actions, a)
		}
		return Step{Kind: Input, Actions: actions}, nil

	case "type", "paste":
		text, err := textArg(rest)
		if err != nil {
			return Step{}, err
		}
		a, err := input.ParseToken(lverb + ":" + text)
		return Step{Kind: Input, Actions: []input.Action{a}}, err

	case "resize":
		a, err := input.ParseToken("resize:" + strings.TrimSpace(rest))
		return Step{Kind: Input, Actions: []input.Action{a}}, err

	case "wait-for", "wait-gone", "assert", "assert-not":
		text, err := textArg(rest)
		if err != nil {
			return Step{}, err
		}
		if text == "" {
			return Step{}, fmt.Errorf("%s needs text", verb)
		}
		kind := map[string]Kind{"wait-for": WaitFor, "wait-gone": WaitGone, "assert": Assert, "assert-not": AssertNot}[lverb]
		return Step{Kind: kind, Arg: text}, nil

	case "assert-style":
		if strings.TrimSpace(rest) == "" {
			return Step{}, fmt.Errorf("assert-style needs a selector and expectations")
		}
		return Step{Kind: AssertStyle, Arg: rest}, nil

	case "wait-stable":
		return Step{Kind: WaitStable}, nil

	case "sleep":
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil {
			return Step{}, fmt.Errorf("sleep: %w", err)
		}
		return Step{Kind: Sleep, Duration: d}, nil

	case "capture":
		name, err := textArg(rest)
		return Step{Kind: Capture, Arg: name}, err
	}

	// Mouse verbs, possibly with modifiers: "shift+click 12,0 right".
	if _, base, _ := splitMods(verb); input.IsMouseVerb(base) {
		a, err := input.ParseToken(verb + ":" + mouseArgs(base, rest))
		return Step{Kind: Input, Actions: []input.Action{a}}, err
	}

	return Step{}, fmt.Errorf("unknown step %q", verb)
}

// textArg removes one pair of surrounding quotes and processes escapes.
func textArg(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	return input.Unescape(s)
}

// mouseArgs turns space-separated mouse arguments into the token form:
// "12,0 right" -> "12,0,right", and "5,3 20,3" for a drag -> "5,3-20,3".
func mouseArgs(verb, rest string) string {
	fields := strings.Fields(rest)
	if strings.EqualFold(verb, "drag") && len(fields) >= 2 &&
		strings.Contains(fields[0], ",") && strings.Contains(fields[1], ",") &&
		!strings.Contains(fields[0], "-") {
		head := fields[0] + "-" + fields[1]
		return strings.Join(append([]string{head}, fields[2:]...), ",")
	}
	return strings.Join(fields, ",")
}

// splitMods strips "mod+" prefixes; it mirrors the key syntax.
func splitMods(s string) (mods, base string, ok bool) {
	rest := s
	for {
		idx := strings.IndexAny(rest, "+-")
		if idx <= 0 {
			break
		}
		switch strings.ToLower(rest[:idx]) {
		case "shift", "alt", "ctrl", "meta":
			rest = rest[idx+1:]
			ok = true
			continue
		}
		break
	}
	return s[:len(s)-len(rest)], rest, ok
}
