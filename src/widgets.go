package src

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// ---- text input ---------------------------------------------------------------

type Input struct {
	R []rune
	C int
}

func (in *Input) Set(s string)   { in.R = []rune(s); in.C = len(in.R) }
func (in *Input) String() string { return string(in.R) }

func (in *Input) ins(s string) {
	s = strings.Replace(strings.Replace(s, "\r", "", -1), "\n", " ", -1)
	rs := []rune(s)
	in.R = concat(in.R[:in.C], rs, in.R[in.C:])
	in.C += len(rs)
}

// Key edits the input; returns true if the key was consumed.
func (in *Input) Key(a *App, ev Event) bool {
	if ev.Key == "" && ev.Rune != 0 {
		in.ins(string(ev.Rune))
		return true
	}
	switch ev.Key {
	case "backspace":
		if in.C > 0 {
			in.R = concat(in.R[:in.C-1], in.R[in.C:])
			in.C--
		}
	case "ctrl+backspace":
		j := in.C
		for j > 0 && in.R[j-1] == ' ' {
			j--
		}
		for j > 0 && in.R[j-1] != ' ' {
			j--
		}
		in.R = concat(in.R[:j], in.R[in.C:])
		in.C = j
	case "delete":
		if in.C < len(in.R) {
			in.R = concat(in.R[:in.C], in.R[in.C+1:])
		}
	case "left":
		if in.C > 0 {
			in.C--
		}
	case "right":
		if in.C < len(in.R) {
			in.C++
		}
	case "ctrl+left":
		for in.C > 0 && in.R[in.C-1] == ' ' {
			in.C--
		}
		for in.C > 0 && in.R[in.C-1] != ' ' {
			in.C--
		}
	case "ctrl+right":
		for in.C < len(in.R) && in.R[in.C] != ' ' {
			in.C++
		}
		for in.C < len(in.R) && in.R[in.C] == ' ' {
			in.C++
		}
	case "home", "ctrl+a":
		in.C = 0
	case "end", "ctrl+e":
		in.C = len(in.R)
	case "ctrl+v", "shift+insert":
		in.ins(a.clipGet())
	case "ctrl+u":
		in.R = in.R[in.C:]
		in.C = 0
	default:
		return false
	}
	return true
}

// drawInput draws a single line input and places the caret when focused.
func (a *App) drawInput(x, y, w int, in *Input, fg, bg Color, focus bool) {
	a.scr.Fill(x, y, w, 1, ' ', fg, bg)
	start := 0
	if in.C >= w-1 {
		start = in.C - w + 2
	}
	for i := 0; i < w && start+i < len(in.R); i++ {
		a.scr.Set(x+i, y, in.R[start+i], fg, bg)
	}
	if focus {
		a.cx, a.cy, a.showCur = x+in.C-start, y, true
	}
}

// ---- fuzzy matching -----------------------------------------------------------

func fuzzy(q, s string) (int, []int, bool) {
	if q == "" {
		return 0, nil, true
	}
	qr := []rune(strings.ToLower(q))
	sr := []rune(s)
	var idx []int
	score := 0
	qi := 0
	prevMatch := -2
	for i, c := range sr {
		if qi < len(qr) && unicode.ToLower(c) == qr[qi] {
			idx = append(idx, i)
			score += 1
			if prevMatch == i-1 {
				score += 8
			}
			if i == 0 || strings.ContainsRune("/\\._- :>", sr[i-1]) {
				score += 6
			}
			prevMatch = i
			qi++
		}
	}
	if qi < len(qr) {
		return 0, nil, false
	}
	score -= len(sr) / 8
	return score, idx, true
}

// ---- overlays -------------------------------------------------------------------

type Overlay interface {
	Key(a *App, ev Event)
	Mouse(a *App, ev Event)
	Draw(a *App)
}

type PItem struct {
	Label, Detail string
	Run           func(a *App)
	Idx           []int
	File          string
	score         int
}

type Palette struct {
	in       Input
	items    []PItem
	sel, top int
	fixed    []PItem
	files    []string
	loaded   bool
	box      Rect
	hint     string
	prevPath string
	prevL    [][]rune
	prevC    [][]Color
	prevMsg  string
}

func (a *App) openPalette(prefix string) {
	p := &Palette{}
	p.in.Set(prefix)
	p.refresh(a)
	a.overlay = p
}

func (a *App) pick(hint string, items []PItem) {
	p := &Palette{fixed: items, hint: hint}
	p.refresh(a)
	a.overlay = p
}

func (p *Palette) refresh(a *App) {
	q := p.in.String()
	var out []PItem
	switch {
	case p.fixed != nil:
		for _, it := range p.fixed {
			if sc, idx, ok := fuzzy(q, it.Label); ok {
				it.Idx, it.score = idx, sc
				out = append(out, it)
			}
		}
	case strings.HasPrefix(q, ">"):
		q = strings.TrimSpace(q[1:])
		for _, c := range cmds {
			c := c
			if sc, idx, ok := fuzzy(q, c.Title); ok {
				out = append(out, PItem{Label: c.Title, Detail: keyFor(c.ID), Idx: idx, score: sc, Run: func(a *App) { c.Run(a) }})
			}
		}
	case strings.HasPrefix(q, "?"):
		q = strings.TrimSpace(q[1:])
		for _, it := range keymapItems() {
			if sc, idx, ok := fuzzy(q, it.Label); ok {
				it.Idx, it.score = idx, sc
				out = append(out, it)
			}
		}
	case strings.HasPrefix(q, ":"):
		n := 0
		for _, ch := range q[1:] {
			if ch >= '0' && ch <= '9' {
				n = n*10 + int(ch-'0')
			}
		}
		d := a.cur()
		if d != nil {
			lbl := "Type a line number to go to (1 - " + itoa(len(d.Lines)) + ")"
			if n > 0 {
				lbl = "Go to line " + itoa(n)
			}
			out = append(out, PItem{Label: lbl, Run: func(a *App) { a.gotoLine(n) }})
		}
		p.items, p.sel, p.top = out, 0, 0
		return
	default:
		if !p.loaded {
			p.files = a.listFiles()
			p.loaded = true
		}
		seen := map[string]bool{}
		if q == "" {
			for i := len(a.docs) - 1; i >= 0; i-- {
				d := a.docs[i]
				if d.Path != "" {
					rel := a.rel(d.Path)
					seen[rel] = true
					path := d.Path
					out = append(out, PItem{Label: rel, File: rel, Detail: "open", Run: func(a *App) { a.openFile(path, 0) }})
				}
			}
		}
		for _, f := range p.files {
			if seen[f] {
				continue
			}
			sc, idx, ok := fuzzy(q, f)
			if !ok {
				continue
			}
			base := filepath.Base(f)
			if _, _, ok2 := fuzzy(q, base); ok2 && q != "" {
				sc += 12
			}
			f := f
			out = append(out, PItem{Label: f, File: f, Idx: idx, score: sc, Run: func(a *App) { a.openFile(filepath.Join(a.root, f), 0) }})
		}
	}
	if q != "" || p.fixed != nil {
		sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	}
	if len(out) > 300 {
		out = out[:300]
	}
	p.items, p.sel, p.top = out, 0, 0
}

func (p *Palette) Key(a *App, ev Event) {
	vis := p.visible(a)
	switch ev.Key {
	case "esc":
		a.overlay = nil
		return
	case "enter":
		if p.sel < len(p.items) {
			it := p.items[p.sel]
			a.overlay = nil
			if it.Run != nil {
				it.Run(a)
			}
		}
		return
	case "down", "ctrl+n", "ctrl+j":
		if p.sel > 0 {
			p.sel--
		} else {
			p.sel = imax(0, len(p.items)-1)
		}
	case "up", "ctrl+k", "ctrl+p":
		if p.sel < len(p.items)-1 {
			p.sel++
		} else {
			p.sel = 0
		}
	case "pgdn":
		p.sel = imax(0, p.sel-vis)
	case "pgup":
		p.sel = imin(len(p.items)-1, p.sel+vis)
	default:
		if p.in.Key(a, ev) {
			p.refresh(a)
		}
		return
	}
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+vis {
		p.top = p.sel - vis + 1
	}
}

// ---- prompt -----------------------------------------------------------------------

type Prompt struct {
	title string
	in    Input
	ok    func(a *App, s string)
	box   Rect
}

func (a *App) prompt(title, initial string, ok func(a *App, s string)) {
	p := &Prompt{title: title, ok: ok}
	p.in.Set(initial)
	a.overlay = p
}

func (p *Prompt) Key(a *App, ev Event) {
	switch ev.Key {
	case "esc":
		a.overlay = nil
	case "enter":
		s := strings.TrimSpace(p.in.String())
		a.overlay = nil
		if s != "" {
			p.ok(a, s)
		}
	default:
		p.in.Key(a, ev)
	}
}

func (p *Prompt) Mouse(a *App, ev Event) {
	if ev.Action == "down" && !p.box.In(ev.X, ev.Y) {
		a.overlay = nil
	}
}

func (p *Prompt) Draw(a *App) {
	w := imin(a.W-4, 70)
	x := (a.W - w) / 2
	p.box = Rect{x, 1, w, 2}
	a.scr.Fill(x, 1, w, 2, ' ', cDim, cBar)
	a.scr.PutN(x+1, 1, w-2, p.title, cDim, cBar)
	a.drawInput(x+1, 2, w-2, &p.in, cFg, cBg, true)
}

// ---- confirm dialog ------------------------------------------------------------------

type Confirm struct {
	msg     []string
	buttons []string
	sel     int
	cb      func(a *App, i int)
	hits    []Rect
	box     Rect
}

func (a *App) confirm(msg string, buttons []string, cb func(a *App, i int)) {
	a.overlay = &Confirm{msg: strings.Split(msg, "\n"), buttons: buttons, cb: cb}
}

func (c *Confirm) done(a *App, i int) {
	a.overlay = nil
	c.cb(a, i)
}

func (c *Confirm) Key(a *App, ev Event) {
	switch ev.Key {
	case "esc":
		c.done(a, -1)
	case "left", "shift+tab":
		c.sel = (c.sel + len(c.buttons) - 1) % len(c.buttons)
	case "right", "tab":
		c.sel = (c.sel + 1) % len(c.buttons)
	case "enter":
		c.done(a, c.sel)
	default:
		if ev.Key == "" && ev.Rune != 0 {
			for i, b := range c.buttons {
				if unicode.ToLower(ev.Rune) == unicode.ToLower([]rune(b)[0]) {
					c.done(a, i)
					return
				}
			}
		}
	}
}

func (c *Confirm) Mouse(a *App, ev Event) {
	if ev.Action != "down" {
		return
	}
	for i, r := range c.hits {
		if r.In(ev.X, ev.Y) {
			c.done(a, i)
			return
		}
	}
	if !c.box.In(ev.X, ev.Y) {
		c.done(a, -1)
	}
}

func (c *Confirm) Draw(a *App) {
	w := 30
	for _, l := range c.msg {
		w = imax(w, len([]rune(l))+6)
	}
	bw := 0
	for _, b := range c.buttons {
		bw += len([]rune(b)) + 4
	}
	w = imin(imax(w, bw+4), a.W)
	h := len(c.msg) + 4
	x, y := (a.W-w)/2, imax(1, (a.H-h)/2-2)
	c.box = Rect{x, y, w, h}
	a.scr.Fill(x, y, w, h, ' ', cFg, cBar)
	for i, l := range c.msg {
		a.scr.PutN(x+3, y+1+i, w-4, l, cFg, cBar)
	}
	bx := x + w - bw - 1
	c.hits = c.hits[:0]
	for i, b := range c.buttons {
		bg := cBg
		if i == c.sel {
			bg = cAccent
		}
		t := "  " + b + "  "
		a.scr.Put(bx, y+h-2, t, cFg, bg)
		c.hits = append(c.hits, Rect{bx, y + h - 2, len([]rune(t)), 1})
		bx += len([]rune(t)) + 2
	}
}

// ---- popup / drop-down menus -----------------------------------------------------------

type MItem struct {
	Label string
	ID    string
	Fn    func(a *App)
	Sep   bool
}

type Menu struct {
	X, Y  int
	Items []MItem
	Sel   int
	Bar   int
	box   Rect
}

type menuDef struct {
	Title string
	Items []MItem
}

var menuBar = []menuDef{
	{"File", []MItem{{"New File", "file.new", nil, false}, {"Open File...", "file.open", nil, false}, {"Open Folder...", "file.openfolder", nil, false}, {"Quick Open...", "file.quickopen", nil, false},
		{"", "", nil, true}, {"Save", "file.save", nil, false}, {"Save As...", "file.saveas", nil, false}, {"Save All", "file.saveall", nil, false},
		{"", "", nil, true}, {"Close Editor", "file.close", nil, false}, {"Close All Editors", "file.closeall", nil, false}, {"", "", nil, true}, {"Exit", "file.exit", nil, false}}},
	{"Edit", []MItem{{"Undo", "edit.undo", nil, false}, {"Redo", "edit.redo", nil, false}, {"", "", nil, true},
		{"Cut", "edit.cut", nil, false}, {"Copy", "edit.copy", nil, false}, {"Paste", "edit.paste", nil, false}, {"", "", nil, true},
		{"Find", "edit.find", nil, false}, {"Replace", "edit.replace", nil, false}, {"Find in Files", "view.search", nil, false}, {"", "", nil, true},
		{"Toggle Line Comment", "edit.comment", nil, false}}},
	{"Selection", []MItem{{"Select All", "edit.selectall", nil, false}, {"Select Line", "sel.line", nil, false}, {"", "", nil, true},
		{"Duplicate Line", "sel.dup", nil, false}, {"Move Line Up", "sel.up", nil, false}, {"Move Line Down", "sel.down", nil, false}, {"Delete Line", "sel.delete", nil, false},
		{"", "", nil, true}, {"Indent Line", "sel.indent", nil, false}, {"Outdent Line", "sel.outdent", nil, false}}},
	{"View", []MItem{{"Command Palette...", "view.palette", nil, false}, {"", "", nil, true},
		{"Explorer", "view.explorer", nil, false}, {"Search", "view.search", nil, false}, {"", "", nil, true},
		{"Toggle Side Bar", "view.sidebar", nil, false}, {"Toggle Panel", "view.panel", nil, false}, {"Terminal", "view.terminal", nil, false}, {"Output", "view.output", nil, false},
		{"", "", nil, true}, {"Window Size...", "view.windowsize", nil, false}, {"", "", nil, true}, {"Color Theme...", "view.theme", nil, false}, {"Keyboard Shortcuts...", "view.keybindings", nil, false}, {"Image Viewer...", "view.image", nil, false}}},
	{"Go", []MItem{{"Go to File...", "file.quickopen", nil, false}, {"Go to Line...", "go.line", nil, false}, {"", "", nil, true},
		{"Next Editor", "go.next", nil, false}, {"Previous Editor", "go.prev", nil, false}}},
	{"Run", []MItem{{"Run File", "run.file", nil, false}, {"Stop", "run.stop", nil, false}}},
	{"Terminal", []MItem{{"Toggle Terminal", "view.terminal", nil, false}, {"Clear Terminal", "term.clear", nil, false}, {"Select Shell...", "term.shell", nil, false}}},
	{"Help", []MItem{{"Keyboard Shortcuts", "view.keybindings", nil, false}, {"Welcome", "help.welcome", nil, false}, {"About Dot", "help.about", nil, false}}},
}

func menuX(i int) int {
	x := 1
	for k := 0; k < i; k++ {
		x += len(menuBar[k].Title) + 2
	}
	return x
}

func (a *App) openMenu(i int) {
	if i < 0 {
		i = len(menuBar) - 1
	}
	i %= len(menuBar)
	m := &Menu{X: menuX(i), Y: 1, Items: menuBar[i].Items, Bar: i}
	a.overlay = m
}

func (a *App) openPopup(x, y int, items []MItem) {
	a.overlay = &Menu{X: x, Y: y, Items: items, Bar: -1}
}

func (m *Menu) run(a *App, i int) {
	it := m.Items[i]
	a.overlay = nil
	if it.Fn != nil {
		it.Fn(a)
	} else if c := cmdByID[it.ID]; c != nil {
		c.Run(a)
	}
}

func (m *Menu) Key(a *App, ev Event) {
	n := len(m.Items)
	step := func(d int) {
		for k := 0; k < n; k++ {
			m.Sel = (m.Sel + d + n) % n
			if !m.Items[m.Sel].Sep {
				return
			}
		}
	}
	switch ev.Key {
	case "esc":
		a.overlay = nil
	case "down":
		step(1)
	case "up":
		step(-1)
	case "enter":
		if !m.Items[m.Sel].Sep {
			m.run(a, m.Sel)
		}
	case "left":
		if m.Bar >= 0 {
			a.openMenu(m.Bar - 1)
		}
	case "right":
		if m.Bar >= 0 {
			a.openMenu(m.Bar + 1)
		}
	}
}

func (m *Menu) Mouse(a *App, ev Event) {
	if ev.Action != "down" {
		return
	}
	if m.box.In(ev.X, ev.Y) {
		i := ev.Y - m.box.Y
		if i >= 0 && i < len(m.Items) && !m.Items[i].Sep {
			m.run(a, i)
		}
		return
	}
	if m.Bar >= 0 && ev.Y == 0 {
		for i := range menuBar {
			if ev.X >= menuX(i) && ev.X < menuX(i)+len(menuBar[i].Title)+2 {
				if i == m.Bar {
					a.overlay = nil
				} else {
					a.openMenu(i)
				}
				return
			}
		}
	}
	a.overlay = nil
}

func (m *Menu) Draw(a *App) {
	w := 10
	for _, it := range m.Items {
		l := len([]rune(it.Label)) + 4
		if k := keyFor(it.ID); k != "" {
			l += len(k) + 3
		}
		w = imax(w, l)
	}
	h := len(m.Items)
	x, y := m.X, m.Y
	if x+w > a.W {
		x = imax(0, a.W-w)
	}
	if y+h > a.H-1 {
		y = imax(0, a.H-1-h)
	}
	m.box = Rect{x, y, w, h}
	if m.Bar >= 0 {
		bx := menuX(m.Bar)
		a.scr.Put(bx, 0, " "+menuBar[m.Bar].Title+" ", cFg, cSel)
	}
	for i, it := range m.Items {
		bg := cBar
		if i == m.Sel && !it.Sep {
			bg = cSel
		}
		a.scr.Fill(x, y+i, w, 1, ' ', cFg, bg)
		if it.Sep {
			a.scr.Fill(x+1, y+i, w-2, 1, '─', cDim, cBar)
			continue
		}
		a.scr.PutN(x+2, y+i, w-3, it.Label, cFg, bg)
		if k := keyFor(it.ID); k != "" {
			a.scr.Put(x+w-len(k)-2, y+i, k, cDim, bg)
		}
	}
}

// ---- helpers ---------------------------------------------------------------------------

func (a *App) listFiles() []string {
	var out []string
	skip := map[string]bool{".git": true, "node_modules": true, "__pycache__": true, ".vs": true, ".idea": true, "$RECYCLE.BIN": true, "System Volume Information": true}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if len(out) > 8000 || depth > 8 {
			return
		}
		ents, err := readDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if e.IsDir() {
				if !skip[e.Name()] {
					walk(filepath.Join(dir, e.Name()), depth+1)
				}
				continue
			}
			out = append(out, a.rel(filepath.Join(dir, e.Name())))
		}
	}
	walk(a.root, 0)
	return out
}

func (a *App) rel(p string) string {
	if r, err := filepath.Rel(a.root, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

type Rect struct{ X, Y, W, H int }

func (r Rect) In(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }
