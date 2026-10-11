package src

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strings"
)

// uni is true when the terminal can show full Unicode (Windows Terminal / VT mode).
var uni bool

type boxChars struct{ tl, tr, bl, br, h, v rune }

func boxStyle() boxChars {
	if uni {
		return boxChars{'╭', '╮', '╰', '╯', '─', '│'}
	}
	return boxChars{'┌', '┐', '└', '┘', '─', '│'}
}

// drawBox draws a bordered, filled window with a title in the top border (Telescope look).
func (a *App) drawBox(x, y, w, h int, title string, border, bg Color) {
	s := a.scr
	b := boxStyle()
	s.Fill(x, y, w, h, ' ', cFg, bg)
	for i := 1; i < w-1; i++ {
		s.Set(x+i, y, b.h, border, bg)
		s.Set(x+i, y+h-1, b.h, border, bg)
	}
	for j := 1; j < h-1; j++ {
		s.Set(x, y+j, b.v, border, bg)
		s.Set(x+w-1, y+j, b.v, border, bg)
	}
	s.Set(x, y, b.tl, border, bg)
	s.Set(x+w-1, y, b.tr, border, bg)
	s.Set(x, y+h-1, b.bl, border, bg)
	s.Set(x+w-1, y+h-1, b.br, border, bg)
	if title != "" {
		s.PutN(x+2, y, w-4, " "+title+" ", cFunc, bg)
	}
}

// ------------------------------------------------------------------ Telescope-style palette

func (p *Palette) title() string {
	q := p.in.String()
	switch {
	case p.fixed != nil && p.hint != "":
		return p.hint
	case strings.HasPrefix(q, ">"):
		return "Commands"
	case strings.HasPrefix(q, "?"):
		return "Keymaps"
	case strings.HasPrefix(q, ":"):
		return "Go to Line"
	}
	return "Find Files"
}

func (p *Palette) geom(a *App) (x, y, tw, th, leftW, vis int) {
	tw = imin(a.W-2, imax(60, a.W*88/100))
	th = imin(a.H-2, imax(12, a.H*72/100))
	x, y = (a.W-tw)/2, (a.H-th)/2
	leftW = tw
	if tw >= 96 {
		leftW = tw * 42 / 100
	}
	vis = imax(1, th-5)
	return
}

func (p *Palette) visible(a *App) int { _, _, _, _, _, v := p.geom(a); return v }

// Draw: LazyVim / Telescope layout - results on top, prompt underneath, preview on the right.
func (p *Palette) Draw(a *App) {
	s := a.scr
	x, y, tw, th, leftW, vis := p.geom(a)
	p.box = Rect{x, y, tw, th}
	resH := th - 3
	a.drawBox(x, y, leftW, resH, "Results", cType, cSide)
	a.drawBox(x, y+resH, leftW, 3, p.title(), cType, cSide)
	py := y + resH + 1
	s.Put(x+2, py, ">", cKw, cSide)
	a.drawInput(x+4, py, leftW-6-10, &p.in, cFg, cSide, true)
	if p.in.String() == "" {
		s.PutN(x+4, py, leftW-16, "type to search   >  commands   ?  keymaps   :  line", cDim, cSide)
	}
	cnt := itoa(len(p.items))
	if len(p.items) > 0 {
		cnt = itoa(p.sel+1) + " / " + cnt
	}
	s.PutN(x+leftW-len(cnt)-3, py, len(cnt), cnt, cDim, cSide)
	if len(p.items) == 0 {
		s.PutN(x+3, y+resH-2, leftW-6, "No matching results", cDim, cSide)
	}
	// results are listed bottom-up: best match sits right above the prompt
	for r := 0; r < vis; r++ {
		i := p.top + r
		if i >= len(p.items) {
			break
		}
		it := p.items[i]
		bg := cSide
		ry := y + resH - 2 - r
		if ry <= y {
			break
		}
		if i == p.sel {
			bg = cSel
			s.Fill(x+1, ry, leftW-2, 1, ' ', cFg, bg)
			s.Set(x+1, ry, '▌', cKw, bg)
		}
		lbl := []rune(it.Label)
		maxl := leftW - 6
		det := it.Detail
		if det != "" {
			maxl -= len([]rune(det)) + 2
		}
		tx := x + 3
		if ic := p.iconFor(a, it); ic.r != 0 {
			s.Set(tx, ry, ic.r, ic.c, bg)
			tx += 2
			maxl -= 2
		}
		mi := 0
		for k := 0; k < len(lbl) && k < maxl; k++ {
			col := cFg
			for mi < len(it.Idx) && it.Idx[mi] < k {
				mi++
			}
			if mi < len(it.Idx) && it.Idx[mi] == k {
				col = cFunc
			}
			s.Set(tx+k, ry, lbl[k], col, bg)
		}
		if det != "" {
			s.PutN(x+leftW-len([]rune(det))-3, ry, len([]rune(det)), det, cKw, bg)
		}
	}
	if leftW < tw {
		p.drawPreview(a, x+leftW, y, tw-leftW, th)
	}
}

func (p *Palette) drawPreview(a *App, x, y, w, h int) {
	s := a.scr
	a.drawBox(x, y, w, h, "Preview", cType, cSide)
	if p.sel >= len(p.items) {
		return
	}
	it := p.items[p.sel]
	if it.File == "" {
		lines := []string{it.Label}
		if it.Detail != "" {
			k := "Key:    "
			if p.fixed != nil {
				k = "Value:  "
			}
			lines = append(lines, "", k+it.Detail)
		}
		for i, ln := range lines {
			col := cFg
			if i == 0 {
				col = cFunc
			}
			s.PutN(x+2, y+1+i, w-4, ln, col, cSide)
		}
		return
	}
	path := filepath.Join(a.root, it.File)
	if p.prevPath != path {
		p.prevPath = path
		p.prevL, p.prevC, p.prevMsg = nil, nil, ""
		data, err := ioutil.ReadFile(path)
		switch {
		case err != nil:
			p.prevMsg = "cannot read file"
		case bytes.IndexByte(data[:imin(len(data), 4096)], 0) >= 0:
			p.prevMsg = "binary file"
		default:
			if len(data) > 100000 {
				data = data[:100000]
			}
			lines, _, _, _ := decodeLines(data)
			if len(lines) > 200 {
				lines = lines[:200]
			}
			lang := langForFile(path)
			var st hlState
			for _, ln := range lines {
				col := make([]Color, len(ln))
				st = lang.scan(ln, st, col)
				p.prevL = append(p.prevL, ln)
				p.prevC = append(p.prevC, col)
			}
		}
	}
	if p.prevMsg != "" {
		s.PutN(x+2, y+1, w-4, p.prevMsg, cDim, cSide)
		return
	}
	for r := 0; r < h-2 && r < len(p.prevL); r++ {
		vx := 0
		for k, ch := range p.prevL[r] {
			if ch == '\t' {
				vx += 4
				continue
			}
			if vx >= w-4 {
				break
			}
			s.Set(x+2+vx, y+1+r, ch, p.prevC[r][k], cSide)
			vx++
		}
	}
}

type iconGlyph struct {
	r rune
	c Color
}

func (p *Palette) iconFor(a *App, it PItem) iconGlyph {
	if p.fixed != nil || it.File == "" {
		return iconGlyph{}
	}
	return fileIcon(it.File)
}

func (p *Palette) Mouse(a *App, ev Event) {
	x, y, _, th, leftW, vis := p.geom(a)
	switch ev.Action {
	case "wheelup":
		p.top = imax(0, imin(len(p.items)-vis, p.top+2))
	case "wheeldown":
		p.top = imax(0, p.top-2)
	case "down":
		if !p.box.In(ev.X, ev.Y) {
			a.overlay = nil
			return
		}
		if ev.X < x || ev.X >= x+leftW {
			return
		}
		r := (y + th - 5) - ev.Y
		if r >= 0 && r < vis && p.top+r < len(p.items) {
			it := p.items[p.top+r]
			a.overlay = nil
			if it.Run != nil {
				it.Run(a)
			}
		}
	}
}

// keymapItems lists every key binding (commands, leader sequences and editing keys).
func keymapItems() []PItem {
	var out []PItem
	pad := func(k string) string { return k + strings.Repeat(" ", imax(1, 24-len([]rune(k)))) }
	seen := map[string]bool{}
	for _, kb := range keymap {
		c := cmdByID[kb.ID]
		if c == nil {
			continue
		}
		out = append(out, PItem{Label: pad(prettyKey(kb.Key)) + c.Title, Run: func(a *App) { c.Run(a) }})
		seen[kb.Key+kb.ID] = true
	}
	var walk func(prefix string, kids []WK)
	walk = func(prefix string, kids []WK) {
		for _, k := range kids {
			if len(k.Kids) > 0 {
				walk(prefix+k.Key+" ", k.Kids)
				continue
			}
			c := cmdByID[k.Cmd]
			if c == nil {
				continue
			}
			out = append(out, PItem{Label: pad("Ctrl+Space "+prefix+k.Key) + k.Label, Run: func(a *App) { c.Run(a) }})
		}
	}
	walk("", leaderTree)
	for _, e := range [][2]string{
		{"Arrows / Home / End", "Move (add Shift to select)"}, {"Ctrl+Left / Right", "Move by word"},
		{"Ctrl+Home / End", "Start / end of file"}, {"Ctrl+Backspace / Delete", "Delete word"},
		{"Tab / Shift+Tab", "Indent / outdent"}, {"PgUp / PgDn", "Page up / down"},
		{"Esc", "Close popup / back to editor"}, {"Mouse drag panel header", "Resize terminal panel"},
		{"Double-click panel header", "Maximize / restore panel"},
	} {
		out = append(out, PItem{Label: pad(e[0]) + e[1]})
	}
	return out
}

// ------------------------------------------------------------------ which-key leader menu

type WK struct {
	Key, Label, Cmd string
	Kids            []WK
}

var leaderTree = []WK{
	{Key: " ", Label: "Find file", Cmd: "file.quickopen"},
	{Key: "/", Label: "Search in files", Cmd: "view.search"},
	{Key: "e", Label: "Explorer", Cmd: "view.explorer"},
	{Key: "p", Label: "Command palette", Cmd: "view.palette"},
	{Key: ",", Label: "Settings", Cmd: "view.settings"},
	{Key: "`", Label: "Terminal", Cmd: "view.terminal"},
	{Key: "r", Label: "Run file", Cmd: "run.file"},
	{Key: "f", Label: "+file", Kids: []WK{
		{Key: "f", Label: "Find file", Cmd: "file.quickopen"},
		{Key: "n", Label: "New file", Cmd: "file.new"},
		{Key: "s", Label: "Save", Cmd: "file.save"},
		{Key: "S", Label: "Save as...", Cmd: "file.saveas"},
		{Key: "a", Label: "Save all", Cmd: "file.saveall"},
		{Key: "o", Label: "Open path...", Cmd: "file.open"},
		{Key: "d", Label: "Open folder...", Cmd: "file.openfolder"},
	}},
	{Key: "s", Label: "+search", Kids: []WK{
		{Key: "g", Label: "Grep in files", Cmd: "view.search"},
		{Key: "f", Label: "Find in file", Cmd: "edit.find"},
		{Key: "r", Label: "Replace", Cmd: "edit.replace"},
		{Key: "l", Label: "Go to line", Cmd: "go.line"},
		{Key: "c", Label: "Commands", Cmd: "view.palette"},
		{Key: "k", Label: "Keymaps", Cmd: "help.keys"},
		{Key: "K", Label: "Edit keyboard shortcuts", Cmd: "view.keybindings"},
	}},
	{Key: "b", Label: "+buffer", Kids: []WK{
		{Key: "n", Label: "Next buffer", Cmd: "go.next"},
		{Key: "p", Label: "Previous buffer", Cmd: "go.prev"},
		{Key: "d", Label: "Close buffer", Cmd: "file.close"},
		{Key: "D", Label: "Close all buffers", Cmd: "file.closeall"},
	}},
	{Key: "c", Label: "+code", Kids: []WK{
		{Key: "r", Label: "Run file", Cmd: "run.file"},
		{Key: "x", Label: "Stop process", Cmd: "run.stop"},
		{Key: "c", Label: "Toggle comment", Cmd: "edit.comment"},
		{Key: "d", Label: "Delete line", Cmd: "sel.delete"},
		{Key: "y", Label: "Duplicate line", Cmd: "sel.dup"},
		{Key: "l", Label: "Change language", Cmd: "editor.lang"},
	}},
	{Key: "u", Label: "+ui", Kids: []WK{
		{Key: "s", Label: "Toggle sidebar", Cmd: "view.sidebar"},
		{Key: "p", Label: "Toggle panel", Cmd: "view.panel"},
		{Key: "t", Label: "Toggle terminal", Cmd: "view.terminal"},
		{Key: "o", Label: "Show output", Cmd: "view.output"},
		{Key: "+", Label: "Panel bigger", Cmd: "view.panelbigger"},
		{Key: "-", Label: "Panel smaller", Cmd: "view.panelsmaller"},
		{Key: "m", Label: "Maximize panel", Cmd: "view.panelmax"},
		{Key: "w", Label: "Window size", Cmd: "view.windowsize"},
		{Key: "i", Label: "Toggle file icons", Cmd: "view.icons"},
		{Key: "c", Label: "Color theme", Cmd: "view.theme"},
		{Key: "k", Label: "Keyboard shortcuts", Cmd: "view.keybindings"},
		{Key: "v", Label: "Image viewer", Cmd: "view.image"},
	}},
	{Key: "t", Label: "+terminal", Kids: []WK{
		{Key: "t", Label: "Toggle terminal", Cmd: "view.terminal"},
		{Key: "c", Label: "Clear terminal", Cmd: "term.clear"},
		{Key: "s", Label: "Select shell", Cmd: "term.shell"},
	}},
	{Key: "h", Label: "+help", Kids: []WK{
		{Key: "k", Label: "Keymaps", Cmd: "help.keys"},
		{Key: "s", Label: "Keyboard shortcuts (edit)", Cmd: "view.keybindings"},
		{Key: "w", Label: "Welcome", Cmd: "help.welcome"},
		{Key: "a", Label: "About", Cmd: "help.about"},
	}},
	{Key: "q", Label: "+quit", Kids: []WK{
		{Key: "q", Label: "Quit Dot", Cmd: "file.exit"},
	}},
}

type WhichKey struct {
	path  []string
	level []WK
	box   Rect
	hits  []Rect
	items []WK
}

func (a *App) openWhichKey() {
	a.overlay = &WhichKey{level: leaderTree}
}

func (w *WhichKey) Key(a *App, ev Event) {
	switch ev.Key {
	case "esc", "ctrl+space", "f4":
		a.overlay = nil
		return
	case "backspace":
		if len(w.path) == 0 {
			a.overlay = nil
			return
		}
		w.path = w.path[:len(w.path)-1]
		w.level = leaderTree
		for _, k := range w.path {
			for _, it := range w.level {
				if it.Key == k {
					w.level = it.Kids
					break
				}
			}
		}
		return
	}
	if ev.Key != "" || ev.Rune == 0 {
		return
	}
	k := string(ev.Rune)
	for _, it := range w.level {
		if it.Key == k {
			w.choose(a, it)
			return
		}
	}
}

func (w *WhichKey) choose(a *App, it WK) {
	if len(it.Kids) > 0 {
		w.path = append(w.path, it.Key)
		w.level = it.Kids
		return
	}
	a.overlay = nil
	if c := cmdByID[it.Cmd]; c != nil {
		c.Run(a)
	}
}

func (w *WhichKey) Mouse(a *App, ev Event) {
	if ev.Action != "down" {
		return
	}
	if !w.box.In(ev.X, ev.Y) {
		a.overlay = nil
		return
	}
	for i, r := range w.hits {
		if r.In(ev.X, ev.Y) && i < len(w.level) {
			w.choose(a, w.level[i])
			return
		}
	}
}

func (w *WhichKey) Draw(a *App) {
	s := a.scr
	colW := 30
	cols := imax(1, (a.W-4)/colW)
	n := len(w.level)
	rows := (n + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	if cols > n {
		cols = n
	}
	bw := imin(a.W-2, cols*colW+3)
	bh := rows + 2
	x, y := (a.W-bw)/2, a.H-1-bh
	w.box = Rect{x, y, bw, bh}
	trail := "Ctrl+Shift+P"
	for _, k := range w.path {
		trail += "  " + k
	}
	a.drawBox(x, y, bw, bh, trail, cKw, cSide)
	if len(w.path) == 0 {
		s.PutN(x+bw-18, y+bh-1, 16, " Esc = close ", cDim, cSide)
	} else {
		s.PutN(x+bw-26, y+bh-1, 24, " Backspace = back ", cDim, cSide)
	}
	arrow := "->"
	if uni {
		arrow = "➜"
	}
	w.hits = w.hits[:0]
	for i, it := range w.level {
		cx := x + 2 + (i/rows)*colW
		cy := y + 1 + i%rows
		w.hits = append(w.hits, Rect{cx, cy, colW - 1, 1})
		key := it.Key
		if key == " " {
			key = "SPC"
		}
		s.PutN(cx, cy, 4, key, cNum, cSide)
		s.PutN(cx+len([]rune(key))+1, cy, 2, arrow, cDim, cSide)
		col := cFg
		if len(it.Kids) > 0 {
			col = cCtl
		}
		s.PutN(cx+len([]rune(key))+4, cy, colW-len([]rune(key))-6, it.Label, col, cSide)
	}
}

// ------------------------------------------------------------------ settings

func (a *App) openSettings() {
	d := a.cur()
	indent, eol, lang, tsz := "-", "-", "-", "-"
	if d != nil {
		indent = "spaces"
		if d.UseTabs {
			indent = "tabs"
		}
		eol = "LF"
		if d.CRLF {
			eol = "CRLF"
		}
		lang = d.Lang.Name
		tsz = itoa(d.TabSize)
	}
	onoff := func(b bool) string {
		if b {
			return "shown"
		}
		return "hidden"
	}
	cmd := func(id string) func(a *App) {
		return func(a *App) {
			if c := cmdByID[id]; c != nil {
				c.Run(a)
			}
		}
	}
	again := func(f func(a *App)) func(a *App) {
		return func(a *App) { f(a); a.saveSettings(); a.openSettings() }
	}
	a.pick("Settings", []PItem{
		{Label: "File icons", Detail: []string{"auto", "on", "off"}[iconMode] + iconNote(), Run: again(cmd("view.icons"))},
		{Label: "Side bar", Detail: onoff(a.sideVis), Run: again(cmd("view.sidebar"))},
		{Label: "Panel (terminal / output)", Detail: onoff(a.panelVis), Run: again(cmd("view.panel"))},
		{Label: "Panel size: bigger", Detail: itoa(a.panelH) + " rows", Run: again(cmd("view.panelbigger"))},
		{Label: "Panel size: smaller", Detail: itoa(a.panelH) + " rows", Run: again(cmd("view.panelsmaller"))},
		{Label: "Side bar width: wider", Detail: itoa(a.sideW), Run: again(cmd("view.sidewider"))},
		{Label: "Side bar width: narrower", Detail: itoa(a.sideW), Run: again(cmd("view.sidenarrower"))},
		{Label: "Window size...", Detail: itoa(a.W) + "x" + itoa(a.H), Run: cmd("view.windowsize")},
		{Label: "Default shell", Detail: a.shell, Run: func(a *App) { cmd("term.shell")(a) }},
		{Label: "Indent using spaces", Detail: indent, Run: again(cmd("editor.spaces"))},
		{Label: "Indent using tabs", Detail: indent, Run: again(cmd("editor.tabs"))},
		{Label: "Tab size...", Detail: tsz, Run: cmd("editor.tabsize")},
		{Label: "End of line (CRLF / LF)", Detail: eol, Run: again(cmd("editor.eol"))},
		{Label: "Language mode...", Detail: lang, Run: cmd("editor.lang")},
		{Label: "Color theme...", Detail: themes[themeIdx].Name, Run: cmd("view.theme")},
		{Label: "Keyboard shortcuts...", Detail: "view / change", Run: cmd("view.keybindings")},
		{Label: "Image viewer...", Detail: "png jpg gif", Run: cmd("view.image")},
		{Label: "Keymaps (list)", Run: cmd("help.keys")},
		{Label: "About Dot", Run: cmd("help.about")},
	})
}
