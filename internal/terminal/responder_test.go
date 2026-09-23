package terminal

import (
	"bytes"
	"sync"
	"testing"
)

func newTestResponder() (*responder, *bytes.Buffer) {
	var replies bytes.Buffer
	r := newResponder(&replies, &sync.Mutex{}, func() (int, int) { return 100, 30 },
		"rgb:ffff/ffff/ffff", "rgb:1e1e/1e1e/2e2e")
	return r, &replies
}

// feed processes data split into the given chunk sizes and returns the
// bytes passed on to the emulator.
func feed(r *responder, data string, chunk int) string {
	var out []byte
	for len(data) > 0 {
		n := min(chunk, len(data))
		out = append(out, r.process([]byte(data[:n]))...)
		data = data[n:]
	}
	return string(out)
}

func TestResponderReplies(t *testing.T) {
	tests := []struct {
		name, in, reply string
	}{
		{"DA1", "\x1b[c", "\x1b[?62;22c"},
		{"DA1 with 0", "\x1b[0c", "\x1b[?62;22c"},
		{"DA2", "\x1b[>c", "\x1b[>1;0;0c"},
		{"XTVERSION", "\x1b[>q", "\x1bP>|tui-goggles\x1b\\"},
		{"XTVERSION with 0", "\x1b[>0q", "\x1bP>|tui-goggles\x1b\\"},
		{"text area size", "\x1b[18t", "\x1b[8;30;100t"},
		{"screen size", "\x1b[19t", "\x1b[9;30;100t"},
		{"pixel size", "\x1b[14t", "\x1b[4;480;800t"},
		{"OSC 11 ST", "\x1b]11;?\x1b\\", "\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\"},
		{"OSC 11 BEL", "\x1b]11;?\a", "\x1b]11;rgb:1e1e/1e1e/2e2e\a"},
		{"OSC 10", "\x1b]10;?\x1b\\", "\x1b]10;rgb:ffff/ffff/ffff\x1b\\"},
		{"OSC 12 cursor", "\x1b]12;?\a", "\x1b]12;rgb:ffff/ffff/ffff\a"},
		{"DECRQM 2026 unsupported", "\x1b[?2026$p", "\x1b[?2026;0$y"},
		{"DECRQM 2027 unsupported", "\x1b[?2027$p", "\x1b[?2027;0$y"},
		{"DECRQM wrap set", "\x1b[?7$p", "\x1b[?7;1$y"},
		{"DECRQM paste reset", "\x1b[?2004$p", "\x1b[?2004;2$y"},
		{"DECRQM ANSI insert", "\x1b[4$p", "\x1b[4;2$y"},
		{"DECRQM ANSI unknown", "\x1b[99$p", "\x1b[99;0$y"},
		{"kitty query unanswered", "\x1b[?u", ""},
		{"OSC 4 query unanswered", "\x1b]4;1;?\a", ""},
	}
	for _, tt := range tests {
		// Every split point must give the same result.
		for chunk := 1; chunk <= len(tt.in); chunk++ {
			r, replies := newTestResponder()
			out := feed(r, "a"+tt.in+"b", chunk)
			if replies.String() != tt.reply {
				t.Errorf("%s (chunk %d): reply %q; want %q", tt.name, chunk, replies.String(), tt.reply)
			}
			if out != "ab" {
				t.Errorf("%s (chunk %d): passed on %q; want the query removed", tt.name, chunk, out)
			}
		}
	}
}

func TestResponderTracksModes(t *testing.T) {
	r, replies := newTestResponder()
	out := feed(r, "\x1b[?2004;1006h\x1b[?25l", 3)
	if out != "\x1b[?2004;1006h\x1b[?25l" {
		t.Errorf("mode sequences should pass through, got %q", out)
	}
	if !r.PrivateMode(2004) || !r.PrivateMode(1006) || r.PrivateMode(25) {
		t.Error("modes not tracked")
	}
	feed(r, "\x1b[?2004$p\x1b[?25$p", 64)
	if got := replies.String(); got != "\x1b[?2004;1$y\x1b[?25;2$y" {
		t.Errorf("mode reports = %q", got)
	}
	feed(r, "\x1bc", 64) // RIS resets
	if r.PrivateMode(2004) || !r.PrivateMode(25) {
		t.Error("RIS did not reset modes")
	}
}

func TestResponderFiltersUnsupported(t *testing.T) {
	for _, in := range []string{
		"\x1b[>1u",     // kitty keyboard push (vt10x: restore cursor)
		"\x1b[<u",      // kitty keyboard pop
		"\x1b[=1;1u",   // kitty keyboard set
		"\x1b[?u",      // kitty keyboard query
		"\x1b[>4;1m",   // modifyOtherKeys (vt10x: SGR reset)
		"\x1b[>4m",     // reset modifyOtherKeys
		"\x1b[?4m",     // XTQMODKEYS
		"\x1b[2 q",     // cursor style
		"\x1b[!p",      // soft reset
		"\x1b[?2026$p", // answered, not passed on
	} {
		r, _ := newTestResponder()
		if out := feed(r, in, 64); out != "" {
			t.Errorf("%q passed on as %q; want dropped", in, out)
		}
	}
}

func TestResponderPassesThrough(t *testing.T) {
	for _, in := range []string{
		"plain text\r\n",
		"\x1b[2J\x1b[H",
		"\x1b[1;31mred\x1b[0m",
		"\x1b[s\x1b[u", // ANSI.SYS save/restore cursor
		"\x1b[?1049h",
		"\x1b]0;title\a",
		"\x1b]11;#ffffff\a", // setting (not querying) a color
		"\x1b[6n",           // DSR: answered by vt10x itself
		"\x1b(B",
		"\x1b[1;2", // incomplete: held, not lost (checked below)
	} {
		r, _ := newTestResponder()
		out := feed(r, in, 64) + string(r.flush())
		if out != in {
			t.Errorf("%q passed on as %q", in, out)
		}
	}
}

func TestNormalizeSGR(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"0", "0"},
		{"1;31", "1;31"},
		{"38;2;255;0;0", "38;2;255;0;0"},
		{"38;5;208;1", "38;5;208;1"},
		{"38:2::255:0:0", "38;2;255;0;0"},
		{"38:2:255:0:0", "38;2;255;0;0"},
		{"48:5:17", "48;5;17"},
		{"4:3", "4"},
		{"4:0", "24"},
		{"1;4:3;38:2::1:2:3", "1;4;38;2;1;2;3"},
		{"58;2;255;0;0;1", "1"},
		{"58:2::255:0:0;1", "1"},
		{"58;5;9;7", "7"},
		{"59;1", "1"},
	}
	for _, tt := range tests {
		if got := normalizeSGR(tt.in); got != tt.want {
			t.Errorf("normalizeSGR(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseColor(t *testing.T) {
	if got, err := ParseColor("#1e1e2e"); err != nil || got != "rgb:1e1e/1e1e/2e2e" {
		t.Errorf("ParseColor = %q, %v", got, err)
	}
	if got, err := ParseColor("ffffff"); err != nil || got != "rgb:ffff/ffff/ffff" {
		t.Errorf("ParseColor without # = %q, %v", got, err)
	}
	for _, bad := range []string{"", "#fff", "#gggggg", "red"} {
		if _, err := ParseColor(bad); err == nil {
			t.Errorf("ParseColor(%q) want error", bad)
		}
	}
}
