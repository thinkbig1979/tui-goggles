package input

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ActionKind identifies what an Action does.
type ActionKind int

const (
	// ActionSend writes Bytes to the application.
	ActionSend ActionKind = iota
	// ActionMouse writes Mouse.Seq to the application.
	ActionMouse
	// ActionPaste writes Bytes (a bracketed paste) to the application.
	ActionPaste
	// ActionResize resizes the terminal to Cols x Rows.
	ActionResize
)

// Action is one parsed -keys token.
type Action struct {
	Kind  ActionKind
	Token string
	Bytes string
	Mouse MouseAction
	Cols  int
	Rows  int
	// ModifyOtherKeys is set for keys that can only be sent in xterm's
	// modifyOtherKeys form; Bytes then holds that form, and it may only be
	// sent once the application has enabled the mode.
	ModifyOtherKeys bool
}

// Paste start and end markers for bracketed paste (mode 2004).
const (
	PasteStart = "\x1b[200~"
	PasteEnd   = "\x1b[201~"
)

// ParseToken parses one -keys token (after Tokenize): type:<text>,
// paste:<text>, a mouse specification, a key specification, or otherwise
// literal text.
func ParseToken(tok string) (Action, error) {
	if text, ok := cutPrefixFold(tok, "type:"); ok {
		return Action{Kind: ActionSend, Token: tok, Bytes: text}, nil
	}
	if text, ok := cutPrefixFold(tok, "paste:"); ok {
		return Action{Kind: ActionPaste, Token: tok, Bytes: PasteStart + text + PasteEnd}, nil
	}
	if size, ok := cutPrefixFold(tok, "resize:"); ok {
		cols, rows, err := ParseSize(size)
		return Action{Kind: ActionResize, Token: tok, Cols: cols, Rows: rows}, err
	}
	if m, ok, err := EncodeMouse(tok); ok {
		return Action{Kind: ActionMouse, Token: tok, Mouse: m}, err
	}
	seq, ok, err := EncodeKey(tok)
	var mok *NeedsModifyOtherKeysError
	if errors.As(err, &mok) {
		return Action{Kind: ActionSend, Token: tok, Bytes: mok.Seq, ModifyOtherKeys: true}, nil
	}
	if err != nil {
		return Action{}, err
	}
	if !ok {
		seq = tok
	}
	return Action{Kind: ActionSend, Token: tok, Bytes: seq}, nil
}

// cutPrefixFold is strings.CutPrefix with a case-insensitive prefix.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// ParseSize parses a terminal size of the form COLSxROWS, e.g. "100x30".
func ParseSize(s string) (cols, rows int, err error) {
	cs, rs, found := strings.Cut(strings.ToLower(s), "x")
	if found {
		cols, err = strconv.Atoi(cs)
		if err == nil {
			rows, err = strconv.Atoi(rs)
		}
	}
	if !found || err != nil || cols < 1 || rows < 1 {
		return 0, 0, fmt.Errorf("invalid size %q (want COLSxROWS, e.g. 100x30)", s)
	}
	return cols, rows, nil
}
