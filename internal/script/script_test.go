package script

import (
	"strings"
	"testing"
	"time"

	"github.com/thinkbig1979/tui-goggles/internal/input"
)

func TestParse(t *testing.T) {
	src := `# a comment
key down  shift+tab

type hello world
type "  padded  "
paste line one\nline two
click 12,0
shift+click 12,0 right
drag 5,3-20,3
drag 5,3 20,3
wheel-down 10,5
resize 100x30
wait-for Saved
wait-gone "Loading..."
wait-stable
sleep 200ms
capture after save
capture
assert Tab 2
assert-not Error
assert-style text="Tab 2" reverse
`
	steps, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 19 {
		t.Fatalf("got %d steps; want 19", len(steps))
	}

	check := func(i int, kind Kind, line int) Step {
		t.Helper()
		s := steps[i]
		if s.Kind != kind || s.Line != line {
			t.Errorf("step %d = kind %d line %d; want kind %d line %d (%q)", i, s.Kind, s.Line, kind, line, s.Text)
		}
		return s
	}

	s := check(0, Input, 2)
	if len(s.Actions) != 2 || s.Actions[1].Bytes != "\x1b[Z" {
		t.Errorf("key step actions = %+v", s.Actions)
	}
	if s := check(1, Input, 4); s.Actions[0].Bytes != "hello world" {
		t.Errorf("type = %q", s.Actions[0].Bytes)
	}
	if s := check(2, Input, 5); s.Actions[0].Bytes != "  padded  " {
		t.Errorf("quoted type = %q", s.Actions[0].Bytes)
	}
	if s := check(3, Input, 6); s.Actions[0].Kind != input.ActionPaste || s.Actions[0].Bytes != "\x1b[200~line one\nline two\x1b[201~" {
		t.Errorf("paste = %+v", s.Actions[0])
	}
	if s := check(4, Input, 7); s.Actions[0].Mouse.Seq != "\x1b[<0;13;1M\x1b[<0;13;1m" {
		t.Errorf("click = %q", s.Actions[0].Mouse.Seq)
	}
	if s := check(5, Input, 8); s.Actions[0].Mouse.Seq != "\x1b[<6;13;1M\x1b[<6;13;1m" {
		t.Errorf("shift+click right = %q", s.Actions[0].Mouse.Seq)
	}
	d1 := check(6, Input, 9).Actions[0].Mouse
	d2 := check(7, Input, 10).Actions[0].Mouse
	if d1.Seq == "" || d1.Seq != d2.Seq {
		t.Errorf("drag forms differ: %q vs %q", d1.Seq, d2.Seq)
	}
	check(8, Input, 11)
	if s := check(9, Input, 12); s.Actions[0].Kind != input.ActionResize || s.Actions[0].Cols != 100 {
		t.Errorf("resize = %+v", s.Actions[0])
	}
	if s := check(10, WaitFor, 13); s.Arg != "Saved" {
		t.Errorf("wait-for arg = %q", s.Arg)
	}
	if s := check(11, WaitGone, 14); s.Arg != "Loading..." {
		t.Errorf("wait-gone arg = %q", s.Arg)
	}
	check(12, WaitStable, 15)
	if s := check(13, Sleep, 16); s.Duration != 200*time.Millisecond {
		t.Errorf("sleep = %v", s.Duration)
	}
	if s := check(14, Capture, 17); s.Arg != "after save" {
		t.Errorf("capture name = %q", s.Arg)
	}
	if s := check(15, Capture, 18); s.Arg != "" {
		t.Errorf("unnamed capture = %q", s.Arg)
	}
	if s := check(16, Assert, 19); s.Arg != "Tab 2" {
		t.Errorf("assert arg = %q", s.Arg)
	}
	check(17, AssertNot, 20)
	if s := check(18, AssertStyle, 21); s.Arg != `text="Tab 2" reverse` {
		t.Errorf("assert-style arg = %q", s.Arg)
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{
		"bogus step",
		"key",
		"key ctrl+nope",
		`key type:"open`,
		"click 1",
		"resize big",
		"sleep soon",
		"wait-for",
		"assert",
		"assert-style",
	} {
		if _, err := Parse(strings.NewReader(src)); err == nil {
			t.Errorf("Parse(%q) succeeded; want error", src)
		} else if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("Parse(%q) error %q does not name the line", src, err)
		}
	}
}
