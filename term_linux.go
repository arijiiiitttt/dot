//go:build linux

package main

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

type unixBackend struct {
	orig   syscall.Termios
	events chan Event
	vtr    vtRenderer
	clip   string
}

func NewBackend(forceVT, forceLegacy bool) Backend { return &unixBackend{} }

func ioctl(req uintptr, p unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, 0, req, uintptr(p))
	if e != 0 {
		return e
	}
	return nil
}

func (b *unixBackend) Init() error {
	if err := ioctl(syscall.TCGETS, unsafe.Pointer(&b.orig)); err != nil {
		return err
	}
	t := b.orig
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB
	t.Cflag |= syscall.CS8
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	ioctl(syscall.TCSETS, unsafe.Pointer(&t))
	os.Stdout.WriteString("\x1b[?1049h\x1b[?7l\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[2J")
	b.events = make(chan Event, 256)
	go b.readLoop()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			b.events <- Event{Kind: evResize}
		}
	}()
	return nil
}

func (b *unixBackend) Close() {
	os.Stdout.WriteString("\x1b[0m\x1b[?1006l\x1b[?1002l\x1b[?1000l\x1b[?25h\x1b[?7h\x1b[?1049l")
	ioctl(syscall.TCSETS, unsafe.Pointer(&b.orig))
}

func (b *unixBackend) Size() (int, int) {
	var ws [4]uint16
	if ioctl(syscall.TIOCGWINSZ, unsafe.Pointer(&ws)) != nil || ws[0] == 0 {
		return 80, 25
	}
	return int(ws[1]), int(ws[0])
}

func (b *unixBackend) Flush(s *Screen, cx, cy int, show bool) {
	os.Stdout.WriteString(b.vtr.Render(s, cx, cy, show))
}
func (b *unixBackend) Events() <-chan Event { return b.events }
func (b *unixBackend) GetClip() (string, bool) { return b.clip, b.clip != "" }
func (b *unixBackend) SetClip(s string)     { b.clip = s }
func (b *unixBackend) Resize(w, h int) bool { return false }

func (b *unixBackend) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return
		}
		b.parse(buf[:n])
	}
}

func modName(m int) (string, bool, bool, bool) {
	m--
	p := ""
	ctrl, alt, shift := m&4 != 0, m&2 != 0, m&1 != 0
	if ctrl {
		p += "ctrl+"
	}
	if alt {
		p += "alt+"
	}
	if shift {
		p += "shift+"
	}
	return p, ctrl, alt, shift
}

func (b *unixBackend) key(name string, mod int) {
	p, c, a, s := "", false, false, false
	if mod > 1 {
		p, c, a, s = modName(mod)
	}
	b.events <- Event{Kind: evKey, Key: p + name, Ctrl: c, Alt: a, Shift: s}
}

func (b *unixBackend) parse(d []byte) {
	for i := 0; i < len(d); {
		c := d[i]
		switch {
		case c == 27:
			if i+1 >= len(d) {
				b.key("esc", 0)
				i++
				continue
			}
			if d[i+1] == '[' || d[i+1] == 'O' {
				j := i + 2
				for j < len(d) && (d[j] < 0x40 || d[j] > 0x7e) {
					j++
				}
				if j >= len(d) {
					return
				}
				seq := string(d[i+2 : j])
				fin := d[j]
				i = j + 1
				b.csi(d[j-len(seq)-1+0:j], seq, fin)
				continue
			}
			// alt+key
			r, sz := utf8.DecodeRune(d[i+1:])
			b.events <- Event{Kind: evKey, Key: "alt+" + string(r), Alt: true}
			i += 1 + sz
		case c == 13:
			b.key("enter", 0)
			i++
		case c == 9:
			b.key("tab", 0)
			i++
		case c == 127:
			b.key("backspace", 0)
			i++
		case c == 8:
			b.key("ctrl+backspace", 0)
			i++
		case c == 0:
			b.events <- Event{Kind: evKey, Key: "ctrl+`", Ctrl: true}
			i++
		case c < 32:
			b.events <- Event{Kind: evKey, Key: "ctrl+" + string(rune('a'+c-1)), Ctrl: true}
			i++
		default:
			r, sz := utf8.DecodeRune(d[i:])
			b.events <- Event{Kind: evKey, Rune: r}
			i += sz
		}
	}
}

func (b *unixBackend) csi(_ []byte, seq string, fin byte) {
	if len(seq) > 0 && seq[0] == '<' && (fin == 'M' || fin == 'm') {
		var v [3]int
		k := 0
		for _, ch := range seq[1:] {
			if ch == ';' {
				k++
				if k > 2 {
					return
				}
				continue
			}
			v[k] = v[k]*10 + int(ch-'0')
		}
		ev := Event{Kind: evMouse, X: v[1] - 1, Y: v[2] - 1, Ctrl: v[0]&16 != 0, Alt: v[0]&8 != 0, Shift: v[0]&4 != 0}
		btn := v[0] & 3
		switch {
		case v[0]&64 != 0:
			if btn == 0 {
				ev.Action = "wheelup"
			} else {
				ev.Action = "wheeldown"
			}
		case v[0]&32 != 0:
			if btn == 0 {
				ev.Action, ev.Btn = "drag", 1
			} else {
				return
			}
		case fin == 'M':
			ev.Action = "down"
			ev.Btn = btn + 1
		default:
			ev.Action = "up"
			ev.Btn = btn + 1
		}
		b.events <- ev
		return
	}
	nums := []int{}
	cur, has := 0, false
	for _, ch := range seq {
		if ch >= '0' && ch <= '9' {
			cur = cur*10 + int(ch-'0')
			has = true
		} else if ch == ';' {
			nums = append(nums, cur)
			cur, has = 0, false
		}
	}
	if has {
		nums = append(nums, cur)
	}
	mod := 0
	if len(nums) >= 2 {
		mod = nums[1]
	}
	switch fin {
	case 'A':
		b.key("up", mod)
	case 'B':
		b.key("down", mod)
	case 'C':
		b.key("right", mod)
	case 'D':
		b.key("left", mod)
	case 'H':
		b.key("home", mod)
	case 'F':
		b.key("end", mod)
	case 'Z':
		b.key("shift+tab", 0)
	case 'P', 'Q', 'R', 'S':
		b.key("f"+strconv.Itoa(int(fin-'P')+1), mod)
	case '~':
		if len(nums) == 0 {
			return
		}
		m := map[int]string{1: "home", 2: "insert", 3: "delete", 4: "end", 5: "pgup", 6: "pgdn", 7: "home", 8: "end",
			15: "f5", 17: "f6", 18: "f7", 19: "f8", 20: "f9", 21: "f10", 23: "f11", 24: "f12"}
		if n, ok := m[nums[0]]; ok {
			b.key(n, mod)
		}
	}
}

func (b *unixBackend) Unicode() bool { return true }
