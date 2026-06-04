package tui

// braillePlane is a subpixel canvas where each terminal cell maps to a 2×4
// dot grid, rendered via Unicode braille (U+2800..U+28FF).
type braillePlane struct {
	wCells, hCells int
	wDots, hDots   int
	dots           []bool
}

func newBraillePlane(wCells, hCells int) *braillePlane {
	wd, hd := wCells*2, hCells*4
	return &braillePlane{
		wCells: wCells, hCells: hCells,
		wDots: wd, hDots: hd,
		dots: make([]bool, wd*hd),
	}
}

func (p *braillePlane) set(dx, dy int) {
	if dx < 0 || dy < 0 || dx >= p.wDots || dy >= p.hDots {
		return
	}
	p.dots[dy*p.wDots+dx] = true
}

// line draws a Bresenham line between two dot coordinates.
func (p *braillePlane) line(x0, y0, x1, y1 int) {
	dx := intAbs(x1 - x0)
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	dy := -intAbs(y1 - y0)
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	for {
		p.set(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// fillBelow fills all dots below the topmost lit dot in each column,
// producing an area-chart effect.
func (p *braillePlane) fillBelow() {
	for x := 0; x < p.wDots; x++ {
		topY := -1
		for y := 0; y < p.hDots; y++ {
			if p.dots[y*p.wDots+x] {
				topY = y
				break
			}
		}
		if topY >= 0 {
			for y := topY + 1; y < p.hDots; y++ {
				p.dots[y*p.wDots+x] = true
			}
		}
	}
}

// cellMask builds the U+2800-relative bitmask for one terminal cell.
func (p *braillePlane) cellMask(cx, cy int) byte {
	type bit struct {
		dx, dy int
		m      byte
	}
	bits := [...]bit{
		{0, 0, 0x01}, {0, 1, 0x02}, {0, 2, 0x04},
		{1, 0, 0x08}, {1, 1, 0x10}, {1, 2, 0x20},
		{0, 3, 0x40}, {1, 3, 0x80},
	}
	var mask byte
	for _, b := range bits {
		dx := cx*2 + b.dx
		dy := cy*4 + b.dy
		if dx >= 0 && dx < p.wDots && dy >= 0 && dy < p.hDots && p.dots[dy*p.wDots+dx] {
			mask |= b.m
		}
	}
	return mask
}

func intAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
