package input

import (
	"fmt"
	"strconv"
	"strings"
)

// SGR (mode 1006) mouse button codes.
const (
	mouseLeft       = 0
	mouseMiddle     = 1
	mouseRight      = 2
	mouseNoButton   = 3
	mouseMotionBit  = 32
	mouseWheelUp    = 64
	mouseWheelDown  = 65
	mouseWheelLeft  = 66
	mouseWheelRight = 67

	mouseModShift = 4
	mouseModAlt   = 8
	mouseModCtrl  = 16
)

// MouseAction is an encoded mouse action.
type MouseAction struct {
	// Seq is the SGR-encoded event sequence, sent in a single write.
	Seq string
	// X and Y are the 0-based target cells; for drags, the end cell.
	X, Y int
	// NeedsButtonMotion is set for actions that report motion with a button
	// held (drag), which requires mode 1002 or 1003.
	NeedsButtonMotion bool
	// NeedsAnyMotion is set for motion without a button (move), which
	// requires mode 1003.
	NeedsAnyMotion bool
}

var mouseVerbs = map[string]bool{
	"click": true, "dblclick": true, "press": true, "release": true,
	"drag": true, "move": true,
	"wheel-up": true, "wheel-down": true, "wheel-left": true, "wheel-right": true,
}

// IsMouseVerb reports whether verb (without modifiers) names a mouse action.
func IsMouseVerb(verb string) bool {
	return mouseVerbs[strings.ToLower(verb)]
}

// EncodeMouse encodes a mouse specification of the form
//
//	[mods+]verb:X,Y[,button]
//	[mods+]drag:X1,Y1-X2,Y2[,button]
//
// where coordinates are 0-based cells, button is left (default), middle or
// right, and mods are shift, alt/meta and ctrl. Verbs are click (press and
// release), dblclick (two clicks), press, release, drag, move (motion with
// no button), wheel-up, wheel-down, wheel-left and wheel-right.
//
// It returns ok=false when spec is not a mouse specification.
func EncodeMouse(spec string) (action MouseAction, ok bool, err error) {
	head, args, found := strings.Cut(spec, ":")
	if !found {
		return action, false, nil
	}
	mods, verb, _ := splitModifiers(head)
	verb = strings.ToLower(verb)
	if !mouseVerbs[verb] {
		return action, false, nil
	}

	modBits := 0
	if mods&ModShift != 0 {
		modBits |= mouseModShift
	}
	if mods&(ModAlt|ModMeta) != 0 {
		modBits |= mouseModAlt
	}
	if mods&ModCtrl != 0 {
		modBits |= mouseModCtrl
	}

	if verb == "drag" {
		action, err = encodeDrag(args, modBits)
		return action, true, err
	}

	parts := strings.Split(args, ",")
	if len(parts) < 2 || len(parts) > 3 {
		return action, true, fmt.Errorf("%s: want X,Y[,button], got %q", verb, args)
	}
	x, y, err := parseCell(parts[0], parts[1])
	if err != nil {
		return action, true, fmt.Errorf("%s: %w", verb, err)
	}
	button := mouseLeft
	if len(parts) == 3 {
		if strings.HasPrefix(verb, "wheel-") || verb == "move" {
			return action, true, fmt.Errorf("%s does not take a button", verb)
		}
		if button, err = parseButton(parts[2]); err != nil {
			return action, true, err
		}
	}
	action.X, action.Y = x, y

	press := sgr(button|modBits, x, y, 'M')
	release := sgr(button|modBits, x, y, 'm')
	switch verb {
	case "click":
		action.Seq = press + release
	case "dblclick":
		action.Seq = press + release + press + release
	case "press":
		action.Seq = press
	case "release":
		action.Seq = release
	case "move":
		action.Seq = sgr(mouseNoButton|mouseMotionBit|modBits, x, y, 'M')
		action.NeedsAnyMotion = true
	case "wheel-up":
		action.Seq = sgr(mouseWheelUp|modBits, x, y, 'M')
	case "wheel-down":
		action.Seq = sgr(mouseWheelDown|modBits, x, y, 'M')
	case "wheel-left":
		action.Seq = sgr(mouseWheelLeft|modBits, x, y, 'M')
	case "wheel-right":
		action.Seq = sgr(mouseWheelRight|modBits, x, y, 'M')
	}
	return action, true, nil
}

// encodeDrag encodes "X1,Y1-X2,Y2[,button]": a press at the start, one
// motion event per cell along the line to the end, and a release at the end.
func encodeDrag(args string, modBits int) (MouseAction, error) {
	var action MouseAction
	from, rest, found := strings.Cut(args, "-")
	if !found {
		return action, fmt.Errorf("drag: want X1,Y1-X2,Y2[,button], got %q", args)
	}
	fromParts := strings.Split(from, ",")
	toParts := strings.Split(rest, ",")
	if len(fromParts) != 2 || len(toParts) < 2 || len(toParts) > 3 {
		return action, fmt.Errorf("drag: want X1,Y1-X2,Y2[,button], got %q", args)
	}
	x1, y1, err := parseCell(fromParts[0], fromParts[1])
	if err != nil {
		return action, fmt.Errorf("drag: %w", err)
	}
	x2, y2, err := parseCell(toParts[0], toParts[1])
	if err != nil {
		return action, fmt.Errorf("drag: %w", err)
	}
	button := mouseLeft
	if len(toParts) == 3 {
		if button, err = parseButton(toParts[2]); err != nil {
			return action, err
		}
	}

	var b strings.Builder
	b.WriteString(sgr(button|modBits, x1, y1, 'M'))
	for _, p := range linePoints(x1, y1, x2, y2)[1:] {
		b.WriteString(sgr(button|mouseMotionBit|modBits, p[0], p[1], 'M'))
	}
	b.WriteString(sgr(button|modBits, x2, y2, 'm'))

	action.Seq = b.String()
	action.X, action.Y = x2, y2
	action.NeedsButtonMotion = true
	return action, nil
}

// sgr formats one SGR mouse event; x and y are 0-based and sent 1-based.
func sgr(code, x, y int, final byte) string {
	return fmt.Sprintf("\x1b[<%d;%d;%d%c", code, x+1, y+1, final)
}

func parseCell(xs, ys string) (int, int, error) {
	x, errX := strconv.Atoi(strings.TrimSpace(xs))
	y, errY := strconv.Atoi(strings.TrimSpace(ys))
	if errX != nil || errY != nil {
		return 0, 0, fmt.Errorf("invalid cell %q,%q", xs, ys)
	}
	if x < 0 || y < 0 {
		return 0, 0, fmt.Errorf("cell %d,%d is negative (coordinates are 0-based)", x, y)
	}
	return x, y, nil
}

func parseButton(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "left", "l":
		return mouseLeft, nil
	case "middle", "m":
		return mouseMiddle, nil
	case "right", "r":
		return mouseRight, nil
	}
	return 0, fmt.Errorf("unknown mouse button %q (want left, middle or right)", s)
}

// linePoints returns the cells on the line from (x1,y1) to (x2,y2),
// inclusive of both ends (Bresenham).
func linePoints(x1, y1, x2, y2 int) [][2]int {
	dx, dy := abs(x2-x1), -abs(y2-y1)
	sx, sy := sign(x2-x1), sign(y2-y1)
	e := dx + dy
	points := [][2]int{{x1, y1}}
	for x1 != x2 || y1 != y2 {
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x1 += sx
		}
		if e2 <= dx {
			e += dx
			y1 += sy
		}
		points = append(points, [2]int{x1, y1})
	}
	return points
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}
