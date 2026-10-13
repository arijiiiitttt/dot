//go:build windows
// +build windows

package src

import (
	"os"
	"os/signal"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	k32 = syscall.NewLazyDLL("kernel32.dll")
	u32 = syscall.NewLazyDLL("user32.dll")

	pGetConsoleMode               = k32.NewProc("GetConsoleMode")
	pSetConsoleMode               = k32.NewProc("SetConsoleMode")
	pReadConsoleInputW            = k32.NewProc("ReadConsoleInputW")
	pCreateConsoleScreenBuffer    = k32.NewProc("CreateConsoleScreenBuffer")
	pSetConsoleActiveScreenBuffer = k32.NewProc("SetConsoleActiveScreenBuffer")
	pGetConsoleScreenBufferInfo   = k32.NewProc("GetConsoleScreenBufferInfo")
	pGetConsoleScreenBufferInfoEx = k32.NewProc("GetConsoleScreenBufferInfoEx")
	pSetConsoleScreenBufferInfoEx = k32.NewProc("SetConsoleScreenBufferInfoEx")
	pSetConsoleScreenBufferSize   = k32.NewProc("SetConsoleScreenBufferSize")
	pSetConsoleWindowInfo         = k32.NewProc("SetConsoleWindowInfo")
	pGetLargestConsoleWindowSize  = k32.NewProc("GetLargestConsoleWindowSize")
	pWriteConsoleOutputW          = k32.NewProc("WriteConsoleOutputW")
	pWriteConsoleW                = k32.NewProc("WriteConsoleW")
	pSetConsoleCursorPosition     = k32.NewProc("SetConsoleCursorPosition")
	pSetConsoleCursorInfo         = k32.NewProc("SetConsoleCursorInfo")
	pSetConsoleTitleW             = k32.NewProc("SetConsoleTitleW")
	pGlobalAlloc                  = k32.NewProc("GlobalAlloc")
	pGlobalLock                   = k32.NewProc("GlobalLock")
	pGlobalUnlock                 = k32.NewProc("GlobalUnlock")
	pOpenClipboard                = u32.NewProc("OpenClipboard")
	pCloseClipboard               = u32.NewProc("CloseClipboard")
	pEmptyClipboard               = u32.NewProc("EmptyClipboard")
	pGetClipboardData             = u32.NewProc("GetClipboardData")
	pSetClipboardData             = u32.NewProc("SetClipboardData")
)

type coord struct{ X, Y int16 }

func (c coord) pack() uintptr { return uintptr(uint32(uint16(c.X)) | uint32(uint16(c.Y))<<16) }

type smallRect struct{ L, T, R, B int16 }
type csbi struct {
	Size   coord
	Cursor coord
	Attr   uint16
	Win    smallRect
	Max    coord
}
type csbiEx struct {
	CbSize     uint32
	Size       coord
	Cursor     coord
	Attr       uint16
	Win        smallRect
	Max        coord
	Popup      uint16
	Fullscreen int32
	Table      [16]uint32
}
type charInfo struct{ Char, Attr uint16 }
type cursorInfo struct {
	Size    uint32
	Visible int32
}
type inputRecord struct {
	Type uint16
	_    uint16
	Data [16]byte
}

type winBackend struct {
	hin, hout, orig syscall.Handle
	inMode, outMode uint32
	vt              bool
	forceVT, forceL bool
	alt             bool
	ci              []charInfo
	vtr             vtRenderer
	events          chan Event
	prevBtn         uint32
}

func NewBackend(forceVT, forceLegacy bool) Backend {
	return &winBackend{forceVT: forceVT, forceL: forceLegacy}
}

func winBuild() int {
	p := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")
	if p.Find() != nil {
		return 0
	}
	var v struct {
		Size                          uint32
		Major, Minor, Build, Platform uint32
		CSD                           [128]uint16
	}
	v.Size = uint32(unsafe.Sizeof(v))
	p.Call(uintptr(unsafe.Pointer(&v)))
	return int(v.Build)
}

func (b *winBackend) Unicode() bool { return b.vt }

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
func le32(b []byte) uint32 { return uint32(le16(b)) | uint32(le16(b[2:]))<<16 }

func (b *winBackend) Init() error {
	b.hin, _ = syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	b.orig, _ = syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	r, _, _ := pGetConsoleMode.Call(uintptr(b.hin), uintptr(unsafe.Pointer(&b.inMode)))
	if r == 0 {
		return os.ErrInvalid
	}
	// mouse | window events | extended flags (quick-edit OFF); no processed input => Ctrl+C is a key
	pSetConsoleMode.Call(uintptr(b.hin), 0x10|0x08|0x80)
	signal.Ignore(os.Interrupt)

	useVT := b.forceVT
	if !b.forceL && !useVT {
		// Windows 10 1703+ (build 15063) understands true-colour ANSI everywhere (conhost and
		// Windows Terminal). Older systems use the classic console API with a redefined palette.
		if winBuild() >= 15063 || os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != "" || os.Getenv("ConEmuANSI") == "ON" {
			useVT = true
		}
	}
	if useVT {
		pGetConsoleMode.Call(uintptr(b.orig), uintptr(unsafe.Pointer(&b.outMode)))
		r, _, _ := pSetConsoleMode.Call(uintptr(b.orig), uintptr(b.outMode|0x1|0x4))
		if r != 0 {
			b.vt = true
			b.hout = b.orig
			b.writeVT("\x1b[?1049h\x1b[?7l\x1b[2J")
		}
	}
	if !b.vt {
		h, _, _ := pCreateConsoleScreenBuffer.Call(0xC0000000, 3, 0, 1, 0)
		if h == ^uintptr(0) || h == 0 {
			b.hout = b.orig
		} else {
			b.hout = syscall.Handle(h)
			b.alt = true
			pSetConsoleActiveScreenBuffer.Call(h)
		}
		b.ApplyPalette(nil)
		// make the window comfortably large if it is the tiny default
		w, h2 := b.Size()
		if w < 100 || h2 < 30 {
			nw, nh := w, h2
			if nw < 110 {
				nw = 110
			}
			if nh < 32 {
				nh = 32
			}
			b.Resize(nw, nh)
		}
	}
	t, _ := syscall.UTF16PtrFromString("Dot")
	pSetConsoleTitleW.Call(uintptr(unsafe.Pointer(t)))
	b.events = make(chan Event, 256)
	go b.readLoop()
	return nil
}

func (b *winBackend) Close() {
	if b.vt {
		b.writeVT("\x1b[0m\x1b[?25h\x1b[?7h\x1b[?1049l")
		pSetConsoleMode.Call(uintptr(b.orig), uintptr(b.outMode))
	} else if b.alt {
		pSetConsoleActiveScreenBuffer.Call(uintptr(b.orig))
		syscall.CloseHandle(b.hout)
	}
	pSetConsoleMode.Call(uintptr(b.hin), uintptr(b.inMode))
}

func (b *winBackend) Size() (int, int) {
	var info csbi
	pGetConsoleScreenBufferInfo.Call(uintptr(b.hout), uintptr(unsafe.Pointer(&info)))
	w := int(info.Win.R-info.Win.L) + 1
	h := int(info.Win.B-info.Win.T) + 1
	if w < 1 || h < 1 {
		return 80, 25
	}
	if !b.vt && (int(info.Size.X) != w || int(info.Size.Y) != h) {
		pSetConsoleScreenBufferSize.Call(uintptr(b.hout), coord{int16(w), int16(h)}.pack())
	}
	return w, h
}

func (b *winBackend) Resize(w, h int) bool {
	if b.vt {
		return false
	}
	r, _, _ := pGetLargestConsoleWindowSize.Call(uintptr(b.hout))
	lx, ly := int(int16(r&0xffff)), int(int16((r>>16)&0xffff))
	if lx > 0 && w > lx {
		w = lx
	}
	if ly > 0 && h > ly {
		h = ly
	}
	var info csbi
	pGetConsoleScreenBufferInfo.Call(uintptr(b.hout), uintptr(unsafe.Pointer(&info)))
	bx, by := int(info.Size.X), int(info.Size.Y)
	if w > bx {
		bx = w
	}
	if h > by {
		by = h
	}
	pSetConsoleScreenBufferSize.Call(uintptr(b.hout), coord{int16(bx), int16(by)}.pack())
	rc := smallRect{0, 0, int16(w - 1), int16(h - 1)}
	pSetConsoleWindowInfo.Call(uintptr(b.hout), 1, uintptr(unsafe.Pointer(&rc)))
	pSetConsoleScreenBufferSize.Call(uintptr(b.hout), coord{int16(w), int16(h)}.pack())
	return true
}

func (b *winBackend) writeVT(s string) {
	u := utf16.Encode([]rune(s))
	for len(u) > 0 {
		n := len(u)
		if n > 8192 {
			n = 8192
		}
		var wr uint32
		pWriteConsoleW.Call(uintptr(b.hout), uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&wr)), 0)
		u = u[n:]
	}
}

func (b *winBackend) Flush(s *Screen, cx, cy int, show bool) {
	if b.vt {
		b.writeVT(b.vtr.Render(s, cx, cy, show))
		return
	}
	n := s.W * s.H
	if len(b.ci) != n {
		b.ci = make([]charInfo, n)
	}
	for i, c := range s.C {
		r := c.R
		if r == 0 || r > 0xFFFF {
			r = ' '
		}
		b.ci[i] = charInfo{uint16(r), uint16(c.Fg) | uint16(c.Bg)<<4}
	}
	rc := smallRect{0, 0, int16(s.W - 1), int16(s.H - 1)}
	pWriteConsoleOutputW.Call(uintptr(b.hout), uintptr(unsafe.Pointer(&b.ci[0])),
		coord{int16(s.W), int16(s.H)}.pack(), 0, uintptr(unsafe.Pointer(&rc)))
	ci := cursorInfo{Size: 25}
	if show {
		ci.Visible = 1
		pSetConsoleCursorPosition.Call(uintptr(b.hout), coord{int16(cx), int16(cy)}.pack())
	}
	pSetConsoleCursorInfo.Call(uintptr(b.hout), uintptr(unsafe.Pointer(&ci)))
}

func (b *winBackend) Events() <-chan Event { return b.events }

func (b *winBackend) readLoop() {
	recs := make([]inputRecord, 64)
	var high uint16
	for {
		var n uint32
		r, _, _ := pReadConsoleInputW.Call(uintptr(b.hin), uintptr(unsafe.Pointer(&recs[0])), 64, uintptr(unsafe.Pointer(&n)))
		if r == 0 {
			return
		}
		for i := 0; i < int(n); i++ {
			rec := &recs[i]
			switch rec.Type {
			case 1:
				d := rec.Data[:]
				if le32(d) == 0 {
					continue
				}
				rep := int(le16(d[4:]))
				if rep < 1 {
					rep = 1
				}
				if rep > 8 {
					rep = 8
				}
				vk, ch, cs := le16(d[6:]), le16(d[10:]), le32(d[12:])
				if ch >= 0xD800 && ch <= 0xDBFF {
					high = ch
					continue
				}
				var rr rune
				if ch >= 0xDC00 && ch <= 0xDFFF && high != 0 {
					rr = utf16.DecodeRune(rune(high), rune(ch))
					high = 0
					ch = 0
				}
				ev, ok := translateKey(vk, ch, cs)
				if rr != 0 {
					ev, ok = Event{Kind: evKey, Rune: rr}, true
				}
				if ok {
					for k := 0; k < rep; k++ {
						b.events <- ev
					}
				}
			case 2:
				d := rec.Data[:]
				x, y := int(int16(le16(d))), int(int16(le16(d[2:])))
				btn, cs, fl := le32(d[4:]), le32(d[8:]), le32(d[12:])
				base := Event{Kind: evMouse, X: x, Y: y, Ctrl: cs&0xC != 0, Alt: cs&0x3 != 0, Shift: cs&0x10 != 0}
				switch {
				case fl&4 != 0:
					if int16(btn>>16) > 0 {
						base.Action = "wheelup"
					} else {
						base.Action = "wheeldown"
					}
					b.events <- base
				case fl&1 != 0:
					if btn&1 != 0 {
						base.Action, base.Btn = "drag", 1
						b.events <- base
					}
				default:
					pressed, released := btn&^b.prevBtn, b.prevBtn&^btn
					if pressed&1 != 0 {
						base.Action, base.Btn, base.Double = "down", 1, fl&2 != 0
						b.events <- base
					}
					if pressed&2 != 0 {
						base.Action, base.Btn = "down", 2
						b.events <- base
					}
					if released&1 != 0 {
						base.Action, base.Btn = "up", 1
						b.events <- base
					}
				}
				b.prevBtn = btn
			case 4:
				b.events <- Event{Kind: evResize}
			}
		}
	}
}

func translateKey(vk, ch uint16, cs uint32) (Event, bool) {
	ctrl := cs&0xC != 0
	alt := cs&0x3 != 0
	shift := cs&0x10 != 0
	if ctrl && alt && ch >= 32 { // AltGr
		ctrl, alt = false, false
	}
	name := ""
	switch vk {
	case 16, 17, 18, 0x5B, 0x5C, 0x90, 0x91, 0x14:
		return Event{}, false
	case 8:
		name = "backspace"
	case 9:
		name = "tab"
	case 13:
		name = "enter"
	case 27:
		name = "esc"
	case 32:
		if ctrl || alt {
			name = "space"
		}
	case 33:
		name = "pgup"
	case 34:
		name = "pgdn"
	case 35:
		name = "end"
	case 36:
		name = "home"
	case 37:
		name = "left"
	case 38:
		name = "up"
	case 39:
		name = "right"
	case 40:
		name = "down"
	case 45:
		name = "insert"
	case 46:
		name = "delete"
	}
	if vk >= 0x70 && vk <= 0x7B {
		name = "f" + itoa(int(vk-0x70)+1)
	}
	if name == "" && (ctrl || alt) {
		switch {
		case vk >= 0x41 && vk <= 0x5A:
			name = string(rune('a' + vk - 0x41))
		case vk >= 0x30 && vk <= 0x39:
			name = string(rune('0' + vk - 0x30))
		default:
			switch vk {
			case 0xC0:
				name = "`"
			case 0xBF:
				name = "/"
			case 0xBB:
				name = "="
			case 0xBD:
				name = "-"
			case 0xBC:
				name = ","
			case 0xBE:
				name = "."
			case 0xBA:
				name = ";"
			case 0xDB:
				name = "["
			case 0xDD:
				name = "]"
			case 0xDC:
				name = "\\"
			case 0xDE:
				name = "'"
			}
		}
	}
	if name == "" && ch == 0x1F { // Windows Terminal sends Ctrl+/ as 0x1F
		name, ctrl = "/", true
	}
	if name != "" {
		p := ""
		if ctrl {
			p += "ctrl+"
		}
		if alt {
			p += "alt+"
		}
		if shift {
			p += "shift+"
		}
		return Event{Kind: evKey, Key: p + name, Ctrl: ctrl, Alt: alt, Shift: shift}, true
	}
	if ch >= 32 && ch != 127 {
		return Event{Kind: evKey, Rune: rune(ch)}, true
	}
	return Event{}, false
}

// ---------------------------------------------------------------- clipboard

func (b *winBackend) SetClip(s string) {
	s = replaceAll(replaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	u := utf16.Encode([]rune(s))
	u = append(u, 0)
	if r, _, _ := pOpenClipboard.Call(0); r == 0 {
		return
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAlloc.Call(2, uintptr(len(u)*2))
	if h == 0 {
		return
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return
	}
	dst := (*[1 << 28]uint16)(unsafe.Pointer(p))[:len(u):len(u)]
	copy(dst, u)
	pGlobalUnlock.Call(h)
	pSetClipboardData.Call(13, h)
}

func (b *winBackend) GetClip() (string, bool) {
	if r, _, _ := pOpenClipboard.Call(0); r == 0 {
		return "", false
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(13)
	if h == 0 {
		return "", false
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return "", false
	}
	defer pGlobalUnlock.Call(h)
	arr := (*[1 << 28]uint16)(unsafe.Pointer(p))
	n := 0
	for n < 1<<27 && arr[n] != 0 {
		n++
	}
	return replaceAll(string(utf16.Decode(arr[:n:n])), "\r\n", "\n"), true
}

// ApplyPalette redefines the console's 16 colours (legacy console only). nil = current theme.
// Not available on Windows XP (no SetConsoleScreenBufferInfoEx): the default colours stay.
func (b *winBackend) ApplyPalette(p *[16]uint32) {
	if b.vt || b.hout == 0 || pGetConsoleScreenBufferInfoEx.Find() != nil {
		return
	}
	if p == nil {
		p = &paletteRGB
	}
	var ex csbiEx
	ex.CbSize = uint32(unsafe.Sizeof(ex))
	pGetConsoleScreenBufferInfoEx.Call(uintptr(b.hout), uintptr(unsafe.Pointer(&ex)))
	for i := 0; i < 16; i++ {
		c := p[i]
		ex.Table[i] = (c>>16)&255 | ((c>>8)&255)<<8 | (c&255)<<16
	}
	ex.Win.R++
	ex.Win.B++
	pSetConsoleScreenBufferInfoEx.Call(uintptr(b.hout), uintptr(unsafe.Pointer(&ex)))
}

// Repaint forces a full redraw after the theme changed.
func (b *winBackend) Repaint() {
	if b.vt {
		b.vtr.Reset()
		return
	}
	b.ApplyPalette(nil)
}

// TrueColor reports whether cells may carry RGB colours (VT mode).
func (b *winBackend) TrueColor() bool { return b.vt }