package src

import (
	"regexp"
	"strconv"
	"unicode/utf8"
)

// ---- multiple terminals ------------------------------------------------------------------

func (a *App) initTerms(root string) {
	a.termSeq = 1
	a.term = NewPane("TERM 1")
	a.term.cwd = root
	a.terms = []*Pane{a.term}
	a.ti = 0
}

// inp is the input line of whichever pane is showing.
func (a *App) inp() *Input {
	if a.panelTab == 1 {
		return &a.term.in
	}
	return &a.out.in
}

func (a *App) anyRunning() bool {
	if a.out.proc != nil {
		return true
	}
	for _, t := range a.terms {
		if t.proc != nil {
			return true
		}
	}
	return false
}

func (a *App) killPane(p *Pane) {
	pr := p.proc
	if pr == nil {
		return
	}
	pr.mu.Lock()
	pr.killed = true
	if pr.cmd != nil && pr.cmd.Process != nil {
		killTree(pr.cmd)
	}
	pr.mu.Unlock()
}

func (a *App) killAll() {
	a.killPane(a.out)
	for _, t := range a.terms {
		a.killPane(t)
	}
}

func (a *App) newTerm() {
	cwd := a.term.cwd
	a.termSeq++
	p := NewPane("TERM " + strconv.Itoa(a.termSeq))
	p.cwd = cwd
	a.terms = append(a.terms, p)
	a.switchTerm(len(a.terms) - 1)
}

func (a *App) switchTerm(i int) {
	if i < 0 || i >= len(a.terms) {
		return
	}
	a.ti = i
	a.term = a.terms[i]
	a.showPanel(1)
}

func (a *App) nextTerm(d int) {
	n := len(a.terms)
	a.switchTerm((a.ti + d + n) % n)
}

// closeTerm kills and removes the terminal at index i (the last one is only cleared and hidden).
func (a *App) closeTerm(i int) {
	if i < 0 || i >= len(a.terms) {
		return
	}
	a.killPane(a.terms[i])
	if len(a.terms) == 1 {
		a.terms[0].Clear()
		a.panelVis = false
		a.focus = fEditor
		return
	}
	a.terms = append(a.terms[:i], a.terms[i+1:]...)
	if a.ti >= len(a.terms) || a.ti > i {
		a.ti--
	}
	if a.ti < 0 {
		a.ti = 0
	}
	a.term = a.terms[a.ti]
	a.panelTab = 1
}

// ---- colouring of output ------------------------------------------------------------------

var (
	reErr   = regexp.MustCompile(`(?i)\b(error|errors:|fatal|exception|traceback|panic|failed|failure|cannot|can't|could not|not found|undefined|denied|segmentation fault|syntaxerror|nameerror|typeerror)\b`)
	reWarn  = regexp.MustCompile(`(?i)\b(warn|warning|warnings|deprecated|caution)\b`)
	reOK    = regexp.MustCompile(`(?i)\b(ok|pass|passed|success|successful|successfully|done|finished|built|compiled|ready|listening|started|running on)\b`)
	reInfo  = regexp.MustCompile(`(?i)^\s*(\[?(info|note|hint|debug|log)\]?)\b`)
	reTrace = regexp.MustCompile(`^\s+(at |File "|from |in |\^|~|\||-->|\d+ \|)`)
	rePath  = regexp.MustCompile(`([A-Za-z]:)?[\\/\w.\-~]+\.\w{1,6}:\d+(:\d+)?|File "[^"]+", line \d+`)
	reURL   = regexp.MustCompile(`https?://[^\s)\]>"']+`)
	reStr   = regexp.MustCompile(`"[^"\n]*"|'[^'\n]*'`)
	reNum   = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
)

func paintSpan(cols []Color, t string, loc []int, c Color) {
	if loc == nil {
		return
	}
	a := utf8.RuneCountInString(t[:loc[0]])
	b := a + utf8.RuneCountInString(t[loc[0]:loc[1]])
	for i := a; i < b && i < len(cols); i++ {
		cols[i] = c
	}
}

// lineColors returns one colour per rune of the line. P > 0 marks a typed command (prompt + command).
func lineColors(ln PLine) []Color {
	t := ln.T
	n := utf8.RuneCountInString(t)
	cols := make([]Color, n)
	base := ln.C
	if ln.P > 0 {
		for i := range cols {
			cols[i] = cFg
		}
		for i := 0; i < ln.P && i < n; i++ {
			cols[i] = cType
		}
		rs := []rune(t)
		i := ln.P
		first := true
		for i < n {
			for i < n && rs[i] == ' ' {
				i++
			}
			if i >= n {
				break
			}
			j := i
			for j < n && rs[j] != ' ' {
				j++
			}
			c := cFg
			switch {
			case first:
				c = cFunc
			case rs[i] == '-':
				c = cVar
			case rs[i] == '"' || rs[i] == '\'':
				c = cStr
			}
			for k := i; k < j; k++ {
				cols[k] = c
			}
			first = false
			i = j
		}
		return cols
	}
	plain := base == cFg
	if plain {
		switch {
		case reErr.MatchString(t):
			base = cRed
		case reWarn.MatchString(t):
			base = cFunc
		case reTrace.MatchString(t):
			base = cDim
		case reInfo.MatchString(t):
			base = cKw
		case reOK.MatchString(t):
			base = cType
		}
	}
	for i := range cols {
		cols[i] = base
	}
	if !plain {
		return cols
	}
	if base == cFg {
		for _, m := range reStr.FindAllStringIndex(t, -1) {
			paintSpan(cols, t, m, cStr)
		}
		for _, m := range reNum.FindAllStringIndex(t, -1) {
			paintSpan(cols, t, m, cNum)
		}
	}
	for _, m := range rePath.FindAllStringIndex(t, -1) {
		paintSpan(cols, t, m, cVar)
	}
	for _, m := range reURL.FindAllStringIndex(t, -1) {
		paintSpan(cols, t, m, cKw)
	}
	return cols
}