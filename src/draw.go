package src

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

func (a *App) hit(x, y, w, h int, fn func()) {
	a.hits = append(a.hits, Hit{Rect{x, y, w, h}, fn})
}

func (a *App) render() {
	a.W, a.H = a.scr.W, a.scr.H
	a.computeLayout()
	a.hits = a.hits[:0]
	a.showCur = false
	s := a.scr
	s.Fill(0, 0, a.W, a.H, ' ', cFg, cBg)
	a.drawTitle()
	a.drawActivity()
	a.drawSidebar()
	if d := a.cur(); d != nil {
		a.drawTabs()
		a.drawCrumbs(d)
		a.drawEditor(d)
		if a.find.open {
			a.drawFind(d)
		}
	} else {
		a.drawTabs()
		a.drawWelcome()
	}
	a.drawPanel()
	a.drawStatus()
	if a.overlay != nil {
		a.showCur = false
		a.overlay.Draw(a)
	}
	a.be.Flush(s, a.cx, a.cy, a.showCur)
}

// ---- title bar & activity bar ---------------------------------------------------------------

func (a *App) drawTitle() {
	s := a.scr
	s.Fill(0, 0, a.W, 1, ' ', cFg, cBar)
	for i, m := range menuBar {
		x := menuX(i)
		s.Put(x, 0, " "+m.Title+" ", cFg, cBar)
		i := i
		a.hit(x, 0, len(m.Title)+2, 1, func() { a.openMenu(i) })
	}
}

func (a *App) drawActivity() {
	defer func() {
		if l := &a.lay; l.actW > 0 {
			y := a.H - 3
			a.scr.Set(1, y, '☼', cDim, cBar) // original char + position
			a.hit(0, y, l.actW, 1, func() { a.openSettings() })
		}
	}()
	l := &a.lay
	if l.actW == 0 {
		return
	}
	s := a.scr
	s.Fill(0, 1, l.actW, a.H-2, ' ', cDim, cBar)
	type ic struct {
		ch     rune
		active bool
		fn     func()
	}
	icons := []ic{
		{'☰', a.sideVis && a.sideView == 0, func() {
			if a.sideVis && a.sideView == 0 {
				a.sideVis = false
			} else {
				a.sideVis, a.sideView, a.focus = true, 0, fSide
			}
		}},
		{'⌕', a.sideVis && a.sideView == 1, func() {
			if a.sideVis && a.sideView == 1 {
				a.sideVis = false
			} else {
				cmdByID["view.search"].Run(a)
			}
		}},
		{'▶', false, func() { a.runFile() }},
		{'›', a.panelVis && a.panelTab == 1, func() {
			cmdByID["view.terminal"].Run(a)
		}},
	}
	for i, ic := range icons {
		y := 2 + i*2
		fg := cDim
		if ic.active {
			fg = cFg
			s.Set(0, y, '▌', cFg, cBar)
		}
		s.Set(1, y, ic.ch, fg, cBar)
		a.hit(0, y, l.actW, 1, ic.fn)
	}
}

// ---- side bar ---------------------------------------------------------------------------------

func (a *App) drawSidebar() {
	l := &a.lay
	if l.sideW == 0 {
		return
	}
	s := a.scr
	x, w := l.sideX, l.sideW
	s.Fill(x, 1, w, a.H-2, ' ', cFg, cSide)
	if a.sideView == 1 {
		a.drawSearch(x, w)
		return
	}
	s.PutN(x+2, 1, w-3, "EXPLORER", cDim, cSide)
	if w >= 16 {
		s.Put(x+w-7, 1, " \u00ab ", cDim, cSide)
		a.hit(x+w-7, 1, 3, 1, func() { a.sideW = imax(10, a.sideW-4) })
		s.Put(x+w-4, 1, " \u00bb ", cDim, cSide)
		a.hit(x+w-4, 1, 3, 1, func() { a.sideW = imin(a.sideW+4, a.W/2) })
	}
	e := a.exp
	h := a.H - 3
	if e.Sel < e.Top {
		e.Top = e.Sel
	}
	if e.Sel >= e.Top+h {
		e.Top = e.Sel - h + 1
	}
	if e.Top < 0 {
		e.Top = 0
	}
	for r := 0; r < h; r++ {
		i := e.Top + r
		if i >= len(e.rows) {
			break
		}
		n := e.rows[i]
		y := 2 + r
		bg := cSide
		if i == e.Sel {
			if a.focus == fSide {
				bg = cSel
			} else {
				bg = cBar
			}
		}
		s.Fill(x, y, w, 1, ' ', cFg, bg)
		ind := n.Depth * 2
		cx := x + 1 + ind
		fg := cFg
		if n.Dir {
			ch := '►'
			if n.Open {
				ch = '▼'
			}
			s.Set(cx, y, ch, cDim, bg)
			name := n.Name
			if n == e.Root {
				name = strings.ToUpper(name)
			}
			if ic := folderIcon(n.Name, n.Open, n == e.Root); ic.r != 0 {
				s.Set(cx+2, y, ic.r, ic.c, bg)
				s.PutN(cx+4, y, x+w-cx-5, fitName(name, x+w-cx-5), fg, bg)
			} else {
				s.PutN(cx+2, y, x+w-cx-3, fitName(name, x+w-cx-3), fg, bg)
			}
		} else {
			ic := fileIcon(n.Name)
			if iconsOn() {
				s.Set(cx+2, y, ic.r, ic.c, bg)
				s.PutN(cx+4, y, x+w-cx-5, fitName(n.Name, x+w-cx-5), fg, bg)
			} else {
				s.Set(cx+1, y, ic.r, ic.c, bg)
				s.PutN(cx+3, y, x+w-cx-4, fitName(n.Name, x+w-cx-4), fg, bg)
			}
			for _, d := range a.docs {
				if d.Path == n.Path && d.Dirty {
					s.Set(x+w-2, y, '•', cFg, bg) // last content column
				}
			}
		}
	}
}

func (a *App) drawSearch(x, w int) {
	s := a.scr
	st := a.srch
	s.PutN(x+2, 1, w-3, "SEARCH", cDim, cSide)
	focus := a.focus == fSide && st.inputFoc
	bg := cBg
	a.drawInput(x+1, 2, w-2, &st.in, cFg, bg, focus)
	if st.in.String() == "" {
		s.PutN(x+1, 2, w-2, " Search (Enter to run)", cDim, bg)
	}
	opt := "Aa"
	fg := cDim
	if st.cs {
		fg = cFg
	}
	s.Put(x+w-4, 3, opt, fg, cSide)
	a.hit(x+w-4, 3, 2, 1, func() { st.cs = !st.cs })
	s.PutN(x+2, 3, w-8, st.status, cDim, cSide)
	h := a.H - 5
	if st.sel < st.top {
		st.top = st.sel
	}
	if st.sel >= st.top+h {
		st.top = st.sel - h + 1
	}
	for r := 0; r < h; r++ {
		i := st.top + r
		if i >= len(st.rows) {
			break
		}
		row := st.rows[i]
		y := 4 + r
		bg := cSide
		if i == st.sel && !st.inputFoc {
			bg = cSel
			if a.focus != fSide {
				bg = cBar
			}
		}
		s.Fill(x, y, w, 1, ' ', cFg, bg)
		if row.Line == 0 {
			s.Put(x+1, y, "▼ ", cDim, bg)
			nx := s.PutN(x+3, y, w-6, filepath.Base(row.Path), cFg, bg)
			s.PutN(nx+1, y, x+w-nx-1, itoa(row.N), cDim, bg)
		} else {
			t := itoa(row.Line) + ": "
			nx := s.PutN(x+2, y, w-3, t, cDim, bg)
			s.PutN(nx, y, x+w-nx-1, row.Text, cFg, bg)
		}
	}
}

// ---- tabs / breadcrumbs ----------------------------------------------------------------------------

func (a *App) drawTabs() {
	l := &a.lay
	s := a.scr
	x0, w := l.edX, l.edW
	bg := cSide
	if len(a.docs) == 0 {
		bg = cBg
	}
	s.Fill(x0, l.tabY, w, 1, ' ', cDim, bg)
	if len(a.docs) == 0 {
		return
	}
	widths := make([]int, len(a.docs))
	for i, d := range a.docs {
		widths[i] = len([]rune(d.Name())) + 7
	}
	if a.tabFirst > a.act {
		a.tabFirst = a.act
	}
	for a.tabFirst < a.act {
		tot := 0
		for i := a.tabFirst; i <= a.act; i++ {
			tot += widths[i]
		}
		if tot <= w {
			break
		}
		a.tabFirst++
	}
	if a.tabFirst >= len(a.docs) {
		a.tabFirst = 0
	}
	x := x0
	for i := a.tabFirst; i < len(a.docs) && x < x0+w; i++ {
		d := a.docs[i]
		bg, fg := cSide, cDim
		if i == a.act {
			bg, fg = cBg, cFg
		}
		tw := imin(widths[i], x0+w-x)
		s.Fill(x, l.tabY, tw, 1, ' ', fg, bg)
		ic := fileIcon(d.Name())
		s.Set(x+1, l.tabY, ic.r, ic.c, bg)
		nameW := tw - 6
		if nameW > 0 {
			s.PutN(x+3, l.tabY, nameW, d.Name(), fg, bg)
		}
		mark, mfg := ' ', cDim
		if d.Dirty {
			mark, mfg = '•', cFg
		} else if i == a.act {
			mark = 'x'
		}
		if tw >= widths[i] {
			s.Set(x+tw-3, l.tabY, mark, mfg, bg)
		}
		i := i
		a.hit(x, l.tabY, tw, 1, func() { a.act = i; a.focus = fEditor })
		if tw >= widths[i] {
			a.hit(x+tw-4, l.tabY, 3, 1, func() { a.closeDoc(i) })
		}
		x += widths[i]
	}
}

func (a *App) drawCrumbs(d *Doc) {
	l := &a.lay
	s := a.scr
	y := l.crumbY
	s.Fill(l.edX, y, l.edW, 1, ' ', cDim, cBg)
	rel := d.Name()
	if d.Path != "" {
		rel = a.rel(d.Path)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	x := l.edX + 2
	for i, p := range parts {
		fg := cDim
		if i == len(parts)-1 {
			fg = cFg
		}
		x = s.PutN(x, y, l.edX+l.edW-x, p, fg, cBg)
		if i < len(parts)-1 {
			x = s.PutN(x, y, l.edX+l.edW-x, " > ", cDim, cBg)
		}
	}
}

// ---- editor -------------------------------------------------------------------------------------------

func (a *App) drawEditor(d *Doc) {
	l := &a.lay
	s := a.scr
	x0, y0, w, h := l.edX, l.bodyY, l.edW, l.bodyH
	gw := gutterW(d)
	tx := x0 + gw
	tw := w - gw - 1
	if tw < 1 {
		return
	}
	sa, sb, hasSel := d.Sel()
	var matches []Match
	curMatch := -1
	if a.find.open {
		matches = a.findMatches()
		curMatch = a.curMatchIndex()
	}
	var marks []int8
	for row := 0; row < h; row++ {
		ln := d.ScrollY + row
		y := y0 + row
		if ln >= len(d.Lines) {
			break
		}
		isCur := ln == d.Cur.L && a.focus != fSide
		bgLine := cBg
		if isCur && !hasSel {
			bgLine = cSide
		}
		s.Fill(tx, y, tw, 1, ' ', cFg, bgLine)
		num := itoa(ln + 1)
		nfg := cDim
		if ln == d.Cur.L {
			nfg = cFg
		}
		s.Put(x0+gw-2-len(num), y, num, nfg, cBg)
		line := d.Lines[ln]
		colors := d.HL(ln)
		if cap(marks) < len(line) {
			marks = make([]int8, len(line)+32)
		}
		marks = marks[:len(line)]
		for i := range marks {
			marks[i] = 0
		}
		if len(matches) > 0 {
			k := sort.Search(len(matches), func(i int) bool { return matches[i].L >= ln })
			for ; k < len(matches) && matches[k].L == ln; k++ {
				m := matches[k]
				v := int8(1)
				if k == curMatch {
					v = 2
				}
				for c := m.C; c < m.C+m.N && c < len(line); c++ {
					marks[c] = v
				}
			}
		}
		leadVis := 0
		lead := leadingWS(line)
		if lead == len(line) {
			lead = 0
		}
		leadVis = d.VisCol(ln, lead)
		vis := 0
		for i := 0; i <= len(line); i++ {
			selected := hasSel && !(Pos{ln, i}).Less(sa) && (Pos{ln, i}).Less(sb)
			if i == len(line) {
				if hasSel && ln >= sa.L && ln < sb.L {
					px := vis - d.ScrollX
					if px >= 0 && px < tw {
						s.Set(tx+px, y, ' ', cFg, cSel)
					}
				}
				break
			}
			r := line[i]
			wdt := 1
			if r == '\t' {
				wdt = d.TabSize - vis%d.TabSize
			}
			fg := colors[i]
			bg := bgLine
			switch {
			case selected:
				bg = cSel
			case marks[i] == 2:
				bg, fg = cFunc, cBg
			case marks[i] == 1:
				bg, fg = cStr, cBg
			}
			for k := 0; k < wdt; k++ {
				px := vis + k - d.ScrollX
				if px < 0 || px >= tw {
					continue
				}
				ch := r
				cfg := fg
				if r == '\t' || (r == ' ' && i < lead) {
					ch = ' '
					if i < lead && (vis+k)%d.TabSize == 0 && vis+k < leadVis && !selected {
						ch, cfg = '│', cBar
					}
				}
				s.Set(tx+px, y, ch, cfg, bg)
			}
			vis += wdt
			if vis-d.ScrollX >= tw+1 {
				break
			}
		}
	}
	// scrollbar
	total := len(d.Lines)
	if total > h {
		th := imax(1, h*h/total)
		ty := y0 + (h-th)*d.ScrollY/imax(1, total-h)
		if ty > y0+h-th {
			ty = y0 + h - th
		}
		for i := 0; i < th; i++ {
			s.Set(x0+w-1, ty+i, '█', cBar, cBg)
		}
	}
	if a.focus == fEditor && a.overlay == nil {
		cx := tx + d.VisCol(d.Cur.L, d.Cur.C) - d.ScrollX
		cy := y0 + d.Cur.L - d.ScrollY
		if cx >= tx && cx < tx+tw+1 && cy >= y0 && cy < y0+h {
			a.cx, a.cy, a.showCur = cx, cy, true
		}
	}
}

func (a *App) drawFind(d *Doc) {
	l := &a.lay
	s := a.scr
	f := a.find
	ww := imin(54, l.edW-4)
	if ww < 24 {
		return
	}
	x := l.edX + l.edW - ww - 2
	y := l.bodyY
	rows := 1
	if f.repl {
		rows = 2
	}
	s.Fill(x, y, ww, rows, ' ', cFg, cBar)
	iw := ww - 24
	if iw < 8 {
		iw = 8
	}
	a.drawInput(x+1, y, iw, &f.q, cFg, cBg, a.focus == fFind && !f.focusR)
	if f.q.String() == "" {
		s.PutN(x+1, y, iw, "Find", cDim, cBg)
	}
	ms := a.findMatches()
	cnt := "No results"
	if f.err != "" {
		cnt = f.err
	} else if f.q.String() == "" {
		cnt = ""
	} else if len(ms) > 0 {
		ci := a.curMatchIndex()
		if ci < 0 {
			cnt = "? of " + itoa(len(ms))
		} else {
			cnt = itoa(ci+1) + " of " + itoa(len(ms))
		}
	}
	cx := x + 2 + iw
	s.PutN(cx, y, 10, cnt, cDim, cBar)
	cx += 11
	tog := func(label string, on bool, fn func()) {
		bg := cBar
		if on {
			bg = cSel
		}
		s.Put(cx, y, label, cFg, bg)
		a.hit(cx, y, len(label), 1, fn)
		cx += len(label) + 1
	}
	tog("Aa", f.cs, func() { f.cs = !f.cs; a.findRefresh(true) })
	tog("W", f.word, func() { f.word = !f.word; a.findRefresh(true) })
	tog(".*", f.re, func() { f.re = !f.re; a.findRefresh(true) })
	s.Put(x+ww-2, y, "x", cDim, cBar)
	a.hit(x+ww-3, y, 3, 1, func() { a.closeFind() })
	if f.repl {
		a.drawInput(x+1, y+1, iw, &f.r, cFg, cBg, a.focus == fFind && f.focusR)
		if f.r.String() == "" {
			s.PutN(x+1, y+1, iw, "Replace", cDim, cBg)
		}
		s.PutN(x+2+iw, y+1, ww-iw-3, "Enter:replace Ctrl+Enter:all", cDim, cBar)
	}
}

// ---- welcome ---------------------------------------------------------------------------------------------

var logo = []string{
	"██████╗  ██████╗ ████████╗",
	"██╔══██╗██╔═══██╗╚══██╔══╝",
	"██║  ██║██║   ██║   ██║   ",
	"██║  ██║██║   ██║   ██║   ",
	"██████╔╝╚██████╔╝   ██║   ",
	"╚═════╝  ╚═════╝    ╚═╝   ",
}

func (a *App) drawWelcome() {
	l := &a.lay
	s := a.scr
	x0, y0, w, h := l.edX, l.tabY+1, l.edW, l.bodyH+2
	s.Fill(x0, y0, w, h, ' ', cFg, cBg)
	type item struct{ t, k, id string }
	items := []item{
		{"New File", "Ctrl+N", "file.new"}, {"Open File...", "Ctrl+P", "file.quickopen"}, {"Open Folder...", "", "file.openfolder"},
		{"Command Palette", "Ctrl+Shift+P", "view.palette"}, {"Keyboard Shortcuts", "", "help.keys"}, {"Toggle Terminal", "Ctrl+`", "view.terminal"},
	}
	need := len(items) + 4
	showLogo := h >= need+len(logo)+4 && w >= 40
	total := need
	if showLogo {
		total += len(logo) + 2
	}
	y := y0 + imax(0, (h-total)/2)
	if showLogo {
		for _, ln := range logo {
			s.Put(x0+(w-len([]rune(ln)))/2, y, ln, cKw, cBg)
			y++
		}
		y++
	}
	t := "Terminal Code Editor"
	s.Put(x0+(w-len(t))/2, y, t, cDim, cBg)
	y += 2
	s.Put(x0+(w-30)/2, y, "Start", cFg, cBg)
	y++
	bx := x0 + imax(1, (w-34)/2)
	for _, it := range items {
		it := it
		s.PutN(bx, y, 34, it.t, cKw, cBg)
		if it.k != "" {
			s.Put(bx+34-len(it.k), y, it.k, cDim, cBg)
		}
		a.hit(bx, y, 34, 1, func() { cmdByID[it.id].Run(a) })
		y++
	}
}

// ---- panel -----------------------------------------------------------------------------------------------

func (a *App) drawPanel() {
	l := &a.lay
	if l.panelH == 0 {
		return
	}
	s := a.scr
	x0, w := l.edX, l.edW
	y := l.panelY
	s.Fill(x0, y, w, 1, ' ', cDim, cSide)
	x := x0 + 1
	limit := x0 + w - 13 - len(a.shell)
	tab := func(label string, active bool, fn func()) {
		if x+len(label)+2 > limit {
			return
		}
		fg, bg := cDim, cSide
		if active {
			fg, bg = cFg, cBg
		}
		s.Put(x, y, " "+label+" ", fg, bg)
		a.hit(x, y, len(label)+2, 1, fn)
		x += len(label) + 3
	}
	tab(map[bool]string{true: "\u25ba OUTPUT", false: "OUTPUT"}[a.out.proc != nil], a.panelTab == 0, func() { a.panelTab = 0; a.focus = fPanel })
	for i, t := range a.terms {
		i, t := i, t
		on := a.panelTab == 1 && i == a.ti
		lab := t.Name
		if t.proc != nil {
			lab = "\u25ba " + lab
		}
		tab(lab, on, func() { a.switchTerm(i); a.focus = fPanel })
		if on && len(a.terms) > 1 {
			x -= 2
			s.Put(x, y, "\u00d7 ", cRed, cBg)
			a.hit(x, y, 2, 1, func() { a.closeTerm(i) })
			x += 2
		}
	}
	if x+3 <= limit {
		s.Put(x, y, " + ", cType, cSide)
		a.hit(x, y, 3, 1, func() { a.newTerm(); a.focus = fPanel })
	}
	right := " x "
	s.Put(x0+w-3, y, right, cDim, cSide)
	a.hit(x0+w-3, y, 3, 1, func() { a.panelVis = false; a.focus = fEditor })
	s.Put(x0+w-6, y, " \u25b2 ", cDim, cSide)
	a.hit(x0+w-6, y, 3, 1, func() { a.setPanelH(a.panelH + 3) })
	s.Put(x0+w-9, y, " \u25bc ", cDim, cSide)
	a.hit(x0+w-9, y, 3, 1, func() { a.setPanelH(a.panelH - 3) })
	if a.panelTab == 1 {
		sh := a.shell
		s.Put(x0+w-11-len(sh), y, sh, cDim, cSide)
	}
	pane := a.out
	if a.panelTab == 1 {
		pane = a.term
	}
	busy := pane.proc != nil
	rows := l.panelH - 1
	lines := pane.Lines
	view := lines[:len(lines)-1]
	last := lines[len(lines)-1]
	var live PLine
	prefix := ""
	hasLive := true
	typing := false
	switch {
	case a.panelTab == 1 && !busy:
		if last.T != "" {
			view = lines
		}
		prefix = a.termPrompt()
		live = PLine{T: prefix + pane.in.String(), C: cFg, P: len([]rune(prefix))}
		typing = true
	case busy:
		prefix = last.T
		live = PLine{T: last.T + pane.in.String(), C: last.C}
		typing = true
	default:
		live = last
		hasLive = last.T != ""
	}
	if a.panelTab == 0 && len(view) == 0 && !hasLive {
		s.Put(x0+2, y+1, "No output yet. Press F5 to run the current file; its output appears here.", cDim, cBg)
		s.Put(x0+2, y+2, "Use the TERMINAL tab for shell commands.", cDim, cBg)
	}
	// long lines wrap to the panel width, so the text always follows the panel size
	nr := func(t string) int {
		n := utf8.RuneCountInString(t)
		if w <= 0 || n <= w {
			return 1
		}
		return (n + w - 1) / w
	}
	total := 0
	for k := range view {
		total += nr(view[k].T)
	}
	if hasLive {
		if typing {
			total++
		} else {
			total += nr(live.T)
		}
	}
	if pane.Scroll > imax(0, total-rows) {
		pane.Scroll = imax(0, total-rows)
	}
	first := imax(0, total-rows-pane.Scroll)
	row := 0
	for k := 0; k <= len(view); k++ {
		var ln PLine
		isLive := false
		h := 1
		if k < len(view) {
			ln = view[k]
			h = nr(ln.T)
		} else if hasLive {
			ln, isLive = live, true
			if !typing {
				h = nr(ln.T)
			}
		} else {
			break
		}
		if row+h <= first {
			row += h
			continue
		}
		if row >= first+rows {
			break
		}
		rs := []rune(ln.T)
		cols := lineColors(ln)
		if isLive && typing {
			yy := y + 1 + row - first
			start := 0
			ci := len([]rune(prefix)) + pane.in.C
			if ci >= w-1 {
				start = ci - w + 2
			}
			if a.focus == fPanel && a.overlay == nil {
				a.cx, a.cy, a.showCur = x0+ci-start, yy, true
			}
			for c := 0; c < w && start+c < len(rs); c++ {
				fg := cFg
				if start+c < len(cols) {
					fg = cols[start+c]
				}
				s.Set(x0+c, yy, rs[start+c], fg, cBg)
			}
			row += h
			continue
		}
		for seg := 0; seg < h; seg++ {
			rr := row + seg
			if rr < first || rr >= first+rows {
				continue
			}
			yy := y + 1 + rr - first
			for c := 0; c < w; c++ {
				idx := seg*w + c
				if idx >= len(rs) {
					break
				}
				fg := cFg
				if idx < len(cols) {
					fg = cols[idx]
				}
				s.Set(x0+c, yy, rs[idx], fg, cBg)
			}
		}
		row += h
	}
}

// ---- status bar ---------------------------------------------------------------------------------------------

func (a *App) drawStatus() {
	s := a.scr
	y := a.lay.statusY
	bg, fg := cAccent, cFg
	if a.toast != "" && a.toastErr {
		bg, fg = cRed, cBg
	}
	s.Fill(0, y, a.W, 1, ' ', fg, bg)
	left := " " + filepath.Base(a.root)
	if a.running() {
		left += "   ► Running..."
	}
	if a.toast != "" {
		left = " " + a.toast
	}
	s.PutN(0, y, a.W, left, fg, bg)
	d := a.cur()
	if d == nil {
		s.Put(a.W-len(" Dot "+version+" "), y, " Dot "+version+" ", fg, bg)
		return
	}
	type seg struct {
		t  string
		fn func()
	}
	pos := "Ln " + itoa(d.Cur.L+1) + ", Col " + itoa(d.VisCol(d.Cur.L, d.Cur.C)+1)
	if t := d.SelText(); t != "" {
		pos += " (" + itoa(len([]rune(t))) + " selected)"
	}
	ind := "Spaces: " + itoa(d.TabSize)
	if d.UseTabs {
		ind = "Tab Size: " + itoa(d.TabSize)
	}
	enc := "UTF-8"
	if d.Latin1 {
		enc = "Windows 1252"
	}
	eol := "LF"
	if d.CRLF {
		eol = "CRLF"
	}
	segs := []seg{
		{pos, func() { a.openPalette(":") }},
		{ind, func() { cmdByID["editor.tabsize"].Run(a) }},
		{enc, nil},
		{eol, func() { cmdByID["editor.eol"].Run(a) }},
		{d.Lang.Name, func() { cmdByID["editor.lang"].Run(a) }},
	}
	if d.ReadOnly {
		segs = append([]seg{{"Read-only", nil}}, segs...)
	}
	x := a.W
	for i := len(segs) - 1; i >= 0; i-- {
		sg := segs[i]
		wd := len([]rune(sg.t)) + 2
		x -= wd
		if x < len([]rune(left))+2 {
			break
		}
		s.Put(x+1, y, sg.t, fg, bg)
		if sg.fn != nil {
			a.hit(x, y, wd, 1, sg.fn)
		}
	}
}

// fitName shortens a name with an ellipsis so it fits in w cells.
func fitName(n string, w int) string {
	rs := []rune(n)
	if w < 2 || len(rs) <= w {
		return n
	}
	return string(rs[:w-1]) + "\u2026"
}