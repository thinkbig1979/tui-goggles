package input

import "testing"

func TestEncodeMouse(t *testing.T) {
	tests := []struct {
		spec string
		want string
	}{
		// Coordinates are 0-based and sent 1-based.
		{"click:0,0", "\x1b[<0;1;1M\x1b[<0;1;1m"},
		{"click:9,4", "\x1b[<0;10;5M\x1b[<0;10;5m"},
		{"click:9,4,right", "\x1b[<2;10;5M\x1b[<2;10;5m"},
		{"click:9,4,middle", "\x1b[<1;10;5M\x1b[<1;10;5m"},
		{"press:3,2", "\x1b[<0;4;3M"},
		{"release:3,2,r", "\x1b[<2;4;3m"},
		{"dblclick:1,1", "\x1b[<0;2;2M\x1b[<0;2;2m\x1b[<0;2;2M\x1b[<0;2;2m"},
		{"wheel-up:5,5", "\x1b[<64;6;6M"},
		{"wheel-down:5,5", "\x1b[<65;6;6M"},
		{"wheel-left:5,5", "\x1b[<66;6;6M"},
		{"wheel-right:5,5", "\x1b[<67;6;6M"},
		{"move:2,3", "\x1b[<35;3;4M"},

		// Modifiers: shift=4, alt/meta=8, ctrl=16
		{"shift+click:0,0", "\x1b[<4;1;1M\x1b[<4;1;1m"},
		{"ctrl+click:0,0", "\x1b[<16;1;1M\x1b[<16;1;1m"},
		{"alt-click:0,0", "\x1b[<8;1;1M\x1b[<8;1;1m"},
		{"ctrl+shift+wheel-down:0,0", "\x1b[<85;1;1M"},

		// Drag: press, motion (+32) per cell after the start, release at the end.
		{"drag:1,0-3,0", "\x1b[<0;2;1M\x1b[<32;3;1M\x1b[<32;4;1M\x1b[<0;4;1m"},
		{"drag:0,0-1,1,right", "\x1b[<2;1;1M\x1b[<34;2;2M\x1b[<2;2;2m"},
		{"shift+drag:0,0-0,1", "\x1b[<4;1;1M\x1b[<36;1;2M\x1b[<4;1;2m"},
	}
	for _, tt := range tests {
		got, ok, err := EncodeMouse(tt.spec)
		if err != nil || !ok || got.Seq != tt.want {
			t.Errorf("EncodeMouse(%q) = %q, ok=%v, err=%v; want %q", tt.spec, got.Seq, ok, err, tt.want)
		}
	}
}

func TestEncodeMouseFlags(t *testing.T) {
	drag, _, _ := EncodeMouse("drag:0,0-5,2")
	if !drag.NeedsButtonMotion || drag.X != 5 || drag.Y != 2 {
		t.Errorf("drag = %+v; want NeedsButtonMotion and end cell 5,2", drag)
	}
	move, _, _ := EncodeMouse("move:1,1")
	if !move.NeedsAnyMotion {
		t.Errorf("move = %+v; want NeedsAnyMotion", move)
	}
}

func TestEncodeMouseNotMouse(t *testing.T) {
	for _, spec := range []string{"click", "hello", "type:hi", "foo:1,2", "12:30"} {
		if _, ok, _ := EncodeMouse(spec); ok {
			t.Errorf("EncodeMouse(%q) ok=true; want not a mouse spec", spec)
		}
	}
}

func TestEncodeMouseErrors(t *testing.T) {
	for _, spec := range []string{
		"click:1", "click:a,b", "click:-1,2", "click:1,2,side", "click:1,2,left,x",
		"wheel-up:1,2,left", "drag:1,2", "drag:1,2-3", "move:1,2,left",
	} {
		if _, ok, err := EncodeMouse(spec); !ok || err == nil {
			t.Errorf("EncodeMouse(%q) ok=%v err=%v; want error", spec, ok, err)
		}
	}
}

func TestLinePoints(t *testing.T) {
	pts := linePoints(0, 0, 3, 1)
	if len(pts) != 4 || pts[0] != [2]int{0, 0} || pts[3] != [2]int{3, 1} {
		t.Errorf("linePoints(0,0,3,1) = %v", pts)
	}
	if pts := linePoints(2, 2, 2, 2); len(pts) != 1 {
		t.Errorf("linePoints of a single cell = %v", pts)
	}
}

func TestParseToken(t *testing.T) {
	a, err := ParseToken("click:1,2")
	if err != nil || a.Kind != ActionMouse {
		t.Errorf("ParseToken(click) = %+v, %v", a, err)
	}
	a, err = ParseToken("alt+left")
	if err != nil || a.Kind != ActionSend || a.Bytes != "\x1b[1;3D" {
		t.Errorf("ParseToken(alt+left) = %+v, %v", a, err)
	}
	a, err = ParseToken("hello")
	if err != nil || a.Bytes != "hello" {
		t.Errorf("ParseToken(hello) = %+v, %v", a, err)
	}
	if _, err = ParseToken("ctrl+nope"); err == nil {
		t.Error("ParseToken(ctrl+nope) want error")
	}
}

func TestParseTokenResize(t *testing.T) {
	a, err := ParseToken("resize:100x30")
	if err != nil || a.Kind != ActionResize || a.Cols != 100 || a.Rows != 30 {
		t.Errorf("resize:100x30 = %+v, %v", a, err)
	}
	for _, bad := range []string{"resize:100", "resize:0x5", "resize:axb", "resize:"} {
		if _, err := ParseToken(bad); err == nil {
			t.Errorf("ParseToken(%q) want error", bad)
		}
	}
}
