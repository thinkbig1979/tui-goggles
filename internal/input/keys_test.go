package input

import "testing"

func TestEncodeKey(t *testing.T) {
	tests := []struct {
		spec string
		want string
	}{
		// Unmodified named keys
		{"up", "\x1b[A"},
		{"Enter", "\r"},
		{"return", "\r"},
		{"tab", "\t"},
		{"esc", "\x1b"},
		{"backspace", "\x7f"},
		{"space", " "},
		{"pgup", "\x1b[5~"},
		{"pagedown", "\x1b[6~"},
		{"delete", "\x1b[3~"},
		{"f1", "\x1bOP"},
		{"f12", "\x1b[24~"},

		// Shift+Tab and aliases
		{"shift+tab", "\x1b[Z"},
		{"shift-tab", "\x1b[Z"},
		{"backtab", "\x1b[Z"},
		{"alt+shift+tab", "\x1b\x1b[Z"},

		// xterm CSI modifiers: param = 1 + shift(1)|alt(2)|ctrl(4)|meta(8)
		{"shift+up", "\x1b[1;2A"},
		{"alt+left", "\x1b[1;3D"},
		{"alt-right", "\x1b[1;3C"},
		{"ctrl+left", "\x1b[1;5D"},
		{"ctrl+shift+right", "\x1b[1;6C"},
		{"ctrl+alt+shift+down", "\x1b[1;8B"},
		{"meta+up", "\x1b[1;9A"},
		{"ctrl+home", "\x1b[1;5H"},
		{"ctrl+end", "\x1b[1;5F"},
		{"shift+end", "\x1b[1;2F"},
		{"ctrl+pgup", "\x1b[5;5~"},
		{"ctrl-pgdn", "\x1b[6;5~"},
		{"ctrl+pgdown", "\x1b[6;5~"},
		{"shift+delete", "\x1b[3;2~"},
		{"ctrl+f1", "\x1b[1;5P"},
		{"shift+f5", "\x1b[15;2~"},

		// Ctrl letters, including the legacy hyphen form
		{"ctrl-a", "\x01"},
		{"ctrl+c", "\x03"},
		{"ctrl-m", "\r"},
		{"CTRL+Z", "\x1a"},
		{"ctrl+space", "\x00"},
		{"ctrl+@", "\x00"},
		{"ctrl+[", "\x1b"},
		{"ctrl+backspace", "\x08"},

		// Alt as ESC prefix
		{"alt+x", "\x1bx"},
		{"alt+X", "\x1bX"},
		{"alt+shift+x", "\x1bX"},
		{"alt+enter", "\x1b\r"},
		{"alt+backspace", "\x1b\x7f"},
		{"ctrl+alt+c", "\x1b\x03"},
		{"alt+-", "\x1b-"},

		// Shift+letter is the upper-case letter
		{"shift+a", "A"},
	}
	for _, tt := range tests {
		got, ok, err := EncodeKey(tt.spec)
		if err != nil || !ok || got != tt.want {
			t.Errorf("EncodeKey(%q) = %q, ok=%v, err=%v; want %q", tt.spec, got, ok, err, tt.want)
		}
	}
}

func TestEncodeKeyLiteral(t *testing.T) {
	// Not key specifications: the caller sends these as literal text.
	for _, spec := range []string{"hello", "q", "1", "-", "a+b", "ctrl-", "foo-bar", "\x1b[Z"} {
		got, ok, err := EncodeKey(spec)
		if ok || err != nil {
			t.Errorf("EncodeKey(%q) = %q, ok=%v, err=%v; want not a key", spec, got, ok, err)
		}
	}
}

func TestEncodeKeyErrors(t *testing.T) {
	for _, spec := range []string{
		"ctrl+foo",    // unknown key with a modifier
		"shift+enter", // no legacy encoding
		"ctrl+tab",
		"ctrl+shift+a",
		"ctrl++",
	} {
		got, ok, err := EncodeKey(spec)
		if err == nil {
			t.Errorf("EncodeKey(%q) = %q, ok=%v; want error", spec, got, ok)
		}
	}
}
