package src

import (
	"strconv"
	"strings"
)

// Palette slots (Windows console order is irrelevant: we redefine all 16 colours).
type Color uint8

const (
	cBg      Color = 0  // editor background  #1e1e1e
	cAccent  Color = 1  // status bar / accent #007acc
	cComment Color = 2  // comments           #6a9955
	cType    Color = 3  // types              #4ec9b0
	cRed     Color = 4  // errors             #f44747
	cCtl     Color = 5  // control keywords   #c586c0
	cFunc    Color = 6  // functions          #dcdcaa
	cFg      Color = 7  // text               #d4d4d4
	cSide    Color = 8  // sidebar bg         #252526
	cKw      Color = 9  // keywords           #569cd6
	cNum     Color = 10 // numbers            #b5cea8
	cVar     Color = 11 // variables          #9cdcfe
	cStr     Color = 12 // strings            #ce9178
	cBar     Color = 13 // title/activity bar #3c3c3c
	cSel     Color = 14 // selection          #264f78
	cDim     Color = 15 // dim text           #858585
)

var paletteRGB = [16]uint32{
	0x1e1e1e, 0x007acc, 0x6a9955, 0x4ec9b0, 0xf44747, 0xc586c0, 0xdcdcaa, 0xd4d4d4,
	0x252526, 0x569cd6, 0xb5cea8, 0x9cdcfe, 0xce9178, 0x3c3c3c, 0x264f78, 0x858585,
}

// Cell is one character cell. FX / BX hold an optional true-colour override (bit 24 set,
// low 24 bits = RGB); they are only used by the image viewer on true-colour terminals.
type Cell struct {
	R      rune
	Fg, Bg Color
	FX, BX uint32
}

const rgbOn = 1 << 24

type Screen struct {
	W, H int
	C    []Cell
}

func NewScreen(w, h int) *Screen {
	s := &Screen{}
	s.Resize(w, h)
	return s
}

func (s *Screen) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	s.W, s.H = w, h
	s.C = make([]Cell, w*h)
}

func (s *Screen) Fill(x, y, w, h int, r rune, fg, bg Color) {
	for j := y; j < y+h; j++ {
		if j < 0 || j >= s.H {
			continue
		}
		for i := x; i < x+w; i++ {
			if i < 0 || i >= s.W {
				continue
			}
			s.C[j*s.W+i] = Cell{R: r, Fg: fg, Bg: bg}
		}
	}
}

func (s *Screen) Set(x, y int, r rune, fg, bg Color) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	s.C[y*s.W+x] = Cell{R: r, Fg: fg, Bg: bg}
}

// SetRGB draws one cell with true-colour foreground and background.
func (s *Screen) SetRGB(x, y int, r rune, fg, bg uint32) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	s.C[y*s.W+x] = Cell{R: r, Fg: cFg, Bg: cBg, FX: fg&0xffffff | rgbOn, BX: bg&0xffffff | rgbOn}
}

// PutN draws str at x,y clipped to maxw cells. Returns x after the last cell drawn.
func (s *Screen) PutN(x, y, maxw int, str string, fg, bg Color) int {
	n := 0
	for _, r := range str {
		if n >= maxw {
			break
		}
		if r < 32 || r == 127 {
			r = ' '
		}
		s.Set(x+n, y, r, fg, bg)
		n++
	}
	return x + n
}

func (s *Screen) Put(x, y int, str string, fg, bg Color) int {
	return s.PutN(x, y, 1<<30, str, fg, bg)
}

// ---------------------------------------------------------------- events

const (
	evKey = iota + 1
	evMouse
	evResize
)

type Event struct {
	Kind   int
	Key    string // "ctrl+s", "up", "enter"... empty for plain runes
	Rune   rune
	X, Y   int
	Btn    int    // 1 left, 2 right
	Action string // down, up, drag, wheelup, wheeldown
	Double bool
	Ctrl   bool
	Shift  bool
	Alt    bool
}

type Backend interface {
	Init() error
	Close()
	Size() (int, int)
	Flush(s *Screen, cx, cy int, show bool)
	Events() <-chan Event
	GetClip() (string, bool)
	SetClip(string)
	Resize(w, h int) bool
}

// ---------------------------------------------------------------- VT renderer

type vtRenderer struct {
	prev   []Cell
	pw, ph int
}

func (v *vtRenderer) Reset() { v.prev = nil }

func (v *vtRenderer) Render(s *Screen, cx, cy int, show bool) string {
	full := v.prev == nil || v.pw != s.W || v.ph != s.H
	var b strings.Builder
	b.WriteString("\x1b[?25l")
	if full {
		b.WriteString("\x1b[2J")
	}
	var lastFg, lastBg uint32 = 1 << 30, 1 << 30
	for y := 0; y < s.H; y++ {
		row := s.C[y*s.W : (y+1)*s.W]
		if !full {
			same := true
			prow := v.prev[y*s.W : (y+1)*s.W]
			for i := range row {
				if row[i] != prow[i] {
					same = false
					break
				}
			}
			if same {
				continue
			}
		}
		b.WriteString("\x1b[" + strconv.Itoa(y+1) + ";1H")
		for _, c := range row {
			f, g := paletteRGB[c.Fg], paletteRGB[c.Bg]
			if c.FX != 0 {
				f = c.FX & 0xffffff
			}
			if c.BX != 0 {
				g = c.BX & 0xffffff
			}
			if f != lastFg || g != lastBg {
				b.WriteString("\x1b[38;2;" + strconv.Itoa(int(f>>16&255)) + ";" + strconv.Itoa(int(f>>8&255)) + ";" + strconv.Itoa(int(f&255)) +
					";48;2;" + strconv.Itoa(int(g>>16&255)) + ";" + strconv.Itoa(int(g>>8&255)) + ";" + strconv.Itoa(int(g&255)) + "m")
				lastFg, lastBg = f, g
			}
			r := c.R
			if r == 0 {
				r = ' '
			}
			b.WriteRune(r)
		}
	}
	if show {
		b.WriteString("\x1b[" + strconv.Itoa(cy+1) + ";" + strconv.Itoa(cx+1) + "H\x1b[?25h")
	}
	v.prev = append(v.prev[:0], s.C...)
	v.pw, v.ph = s.W, s.H
	return b.String()
}
