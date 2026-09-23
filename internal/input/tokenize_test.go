package input

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"down down enter", []string{"down", "down", "enter"}},
		{"  down \t enter  ", []string{"down", "enter"}},
		{"", nil},
		{`type:"hello world" enter`, []string{"type:hello world", "enter"}},
		{`type:'hello world'`, []string{"type:hello world"}},
		{`paste:"a b" "x y"`, []string{"paste:a b", "x y"}},
		{`type:a\sb`, []string{"type:a b"}},
		{`type:a\tb\nc\r`, []string{"type:a\tb\nc\r"}},
		{`\e[Z`, []string{"\x1b[Z"}},
		{`type:\x1b[1;3D`, []string{"type:\x1b[1;3D"}},
		{`type:"say \"hi\""`, []string{`type:say "hi"`}},
		{`type:"it's"`, []string{"type:it's"}},
		{`type:'a\'b'`, []string{"type:a'b"}},
		{`type:c:\\dir`, []string{`type:c:\dir`}},
		{`type:\q`, []string{`type:\q`}}, // unknown escape kept
		{`type:""`, []string{"type:"}},
		{`""`, []string{""}},
	}
	for _, tt := range tests {
		got, err := Tokenize(tt.in)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Tokenize(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestTokenizeErrors(t *testing.T) {
	for _, in := range []string{`type:"unterminated`, `'x`, `\x1`, `\xZZ`} {
		if got, err := Tokenize(in); err == nil {
			t.Errorf("Tokenize(%q) = %q; want error", in, got)
		}
	}
}

func TestUnescape(t *testing.T) {
	got, err := Unescape(`a "b" \t\e`)
	if err != nil || got != "a \"b\" \t\x1b" {
		t.Errorf("Unescape = %q, %v", got, err)
	}
}

func TestParseTokenTypeAndPaste(t *testing.T) {
	a, err := ParseToken("type:hello world")
	if err != nil || a.Kind != ActionSend || a.Bytes != "hello world" {
		t.Errorf("type: = %+v, %v", a, err)
	}
	// type: bypasses key-name parsing
	a, _ = ParseToken("type:enter")
	if a.Bytes != "enter" {
		t.Errorf("type:enter = %q; want literal text", a.Bytes)
	}
	a, err = ParseToken("paste:line1\nline2")
	if err != nil || a.Kind != ActionPaste || a.Bytes != "\x1b[200~line1\nline2\x1b[201~" {
		t.Errorf("paste: = %+v, %v", a, err)
	}
	a, _ = ParseToken("TYPE:x")
	if a.Bytes != "x" {
		t.Errorf("TYPE:x = %q; want case-insensitive prefix", a.Bytes)
	}
}
