package input

// ActionKind identifies what an Action does.
type ActionKind int

const (
	// ActionSend writes Bytes to the application.
	ActionSend ActionKind = iota
	// ActionMouse writes Mouse.Seq to the application.
	ActionMouse
)

// Action is one parsed -keys token.
type Action struct {
	Kind  ActionKind
	Token string
	Bytes string
	Mouse MouseAction
}

// ParseToken parses one -keys token: a mouse specification, a key
// specification, or otherwise literal text.
func ParseToken(tok string) (Action, error) {
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
