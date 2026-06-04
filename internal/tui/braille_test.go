package tui

import "testing"

func TestBraillePlane_Set(t *testing.T) {
	p := newBraillePlane(2, 1)
	if p.wDots != 4 || p.hDots != 4 {
		t.Fatalf("expected 4x4 dots, got %dx%d", p.wDots, p.hDots)
	}
	p.set(0, 0)
	if !p.dots[0] {
		t.Error("dot at (0,0) should be set")
	}
	p.set(-1, 0) // out of bounds, should not panic
	p.set(0, 99) // out of bounds, should not panic
}

func TestBraillePlane_CellMask(t *testing.T) {
	p := newBraillePlane(1, 1)
	// Set bottom-left dot
	p.set(0, 3)
	mask := p.cellMask(0, 0)
	if mask != 0x40 {
		t.Errorf("bottom-left dot should be 0x40, got 0x%02x", mask)
	}
	// Set all dots
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			p.set(x, y)
		}
	}
	mask = p.cellMask(0, 0)
	if mask != 0xFF {
		t.Errorf("all dots should be 0xFF, got 0x%02x", mask)
	}
}

func TestBraillePlane_Line(t *testing.T) {
	p := newBraillePlane(3, 1)
	p.line(0, 2, 5, 2) // horizontal line
	for x := 0; x <= 5; x++ {
		if !p.dots[2*p.wDots+x] {
			t.Errorf("dot at (%d, 2) should be set", x)
		}
	}
}

func TestBraillePlane_FillBelow(t *testing.T) {
	p := newBraillePlane(1, 1)
	p.set(0, 1) // set dot at row 1
	p.fillBelow()
	if !p.dots[1*p.wDots+0] {
		t.Error("original dot should still be set")
	}
	if !p.dots[2*p.wDots+0] {
		t.Error("row 2 should be filled")
	}
	if !p.dots[3*p.wDots+0] {
		t.Error("row 3 should be filled")
	}
	if p.dots[0*p.wDots+0] {
		t.Error("row 0 above the dot should not be filled")
	}
}
