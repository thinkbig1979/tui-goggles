package input

import (
	"errors"
	"testing"
)

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

func TestEncodeKeyModifyOtherKeysFallback(t *testing.T) {
	tests := map[string]string{
		"shift+enter":      "\x1b[27;2;13~",
		"ctrl+enter":       "\x1b[27;5;13~",
		"ctrl+alt+enter":   "\x1b[27;7;13~",
		"ctrl+tab":         "\x1b[27;5;9~",
		"ctrl+shift+tab":   "\x1b[27;6;9~",
		"ctrl+esc":         "\x1b[27;5;27~",
		"shift+backspace":  "\x1b[27;2;127~",
		"ctrl+shift+space": "\x1b[27;6;32~",
		"ctrl+shift+a":     "\x1b[27;6;97~", // unshifted code for letters
		"ctrl+shift+A":     "\x1b[27;6;97~",
		"ctrl+1":           "\x1b[27;5;49~",
		"ctrl+.":           "\x1b[27;5;46~",
	}
	for spec, want := range tests {
		_, ok, err := EncodeKey(spec)
		var mok *NeedsModifyOtherKeysError
		if !ok || !errors.As(err, &mok) || mok.Seq != want {
			t.Errorf("EncodeKey(%q) err=%v; want modifyOtherKeys fallback %q", spec, err, want)
		}
	}
	// Unknown keys are still plain errors.
	var mok *NeedsModifyOtherKeysError
	if _, _, err := EncodeKey("ctrl+foo"); err == nil || errors.As(err, &mok) {
		t.Errorf("EncodeKey(ctrl+foo) err=%v; want a plain error", err)
	}
}

func TestParseTokenModifyOtherKeys(t *testing.T) {
	a, err := ParseToken("shift+enter")
	if err != nil || !a.ModifyOtherKeys || a.Bytes != "\x1b[27;2;13~" {
		t.Errorf("ParseToken(shift+enter) = %+v, %v", a, err)
	}
	// Keys with a legacy encoding keep it.
	a, _ = ParseToken("ctrl+a")
	if a.ModifyOtherKeys || a.Bytes != "\x01" {
		t.Errorf("ParseToken(ctrl+a) = %+v", a)
	}
}
