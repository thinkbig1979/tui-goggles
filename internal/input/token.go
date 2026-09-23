package input

import "strings"

// ActionKind identifies what an Action does.
type ActionKind int

const (
	// ActionSend writes Bytes to the application.
	ActionSend ActionKind = iota
	// ActionMouse writes Mouse.Seq to the application.
	ActionMouse
	// ActionPaste writes Bytes (a bracketed paste) to the application.
	ActionPaste
)

// Action is one parsed -keys token.
type Action struct {
	Kind  ActionKind
	Token string
	Bytes string
	Mouse MouseAction
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
	if m, ok, err := EncodeMouse(tok); ok {
		return Action{Kind: ActionMouse, Token: tok, Mouse: m}, err
	}
	seq, ok, err := EncodeKey(tok)
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
