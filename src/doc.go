package src

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func itoa(n int) string { return strconv.Itoa(n) }
func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type Pos struct{ L, C int }

func (a Pos) Less(b Pos) bool { return a.L < b.L || (a.L == b.L && a.C < b.C) }

type snap struct {
	lines       [][]rune
	cur, anchor Pos
}

type Doc struct {
	Lines    [][]rune
	Path     string
	Title    string
	Untitled int
	Dirty    bool
	CRLF     bool
	BOM      bool
	Latin1   bool
	UseTabs  bool
	TabSize  int
	Lang     *Lang
	Cur      Pos
	Anchor   Pos
	WantVis  int
	ScrollY  int
	ScrollX  int
	Version  int
	ReadOnly bool

	undo, redo []snap
	lastKind   string
	lastEdit   time.Time
	hlStates   []hlState
	hlValid    int
	hlBuf      []Color
}

func NewDoc() *Doc {
	d := &Doc{Lines: [][]rune{{}}, TabSize: 4, WantVis: -1}
	d.Lang = langByName("Plain Text")
	if osIsWindows() {
		d.CRLF = true
	}
	return d
}

func osIsWindows() bool { return filepath.Separator == '\\' }

func (d *Doc) Name() string {
	if d.Title != "" {
		return d.Title
	}
	if d.Path != "" {
		return filepath.Base(d.Path)
	}
	if d.Untitled > 0 {
		return "Untitled-" + itoa(d.Untitled)
	}
	return "Untitled"
}

func decodeLines(b []byte) (lines [][]rune, crlf, bom, latin1 bool) {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		bom = true
		b = b[3:]
	}
	latin1 = !utf8.Valid(b)
	n := bytes.Count(b, []byte("\n"))
	cr := bytes.Count(b, []byte("\r\n"))
	crlf = n > 0 && cr*2 >= n
	lines = make([][]rune, 0, n+1)
	for len(b) >= 0 {
		i := bytes.IndexByte(b, '\n')
		var ln []byte
		if i < 0 {
			ln = b
		} else {
			ln = b[:i]
		}
		if len(ln) > 0 && ln[len(ln)-1] == '\r' {
			ln = ln[:len(ln)-1]
		}
		var r []rune
		if latin1 {
			r = make([]rune, len(ln))
			for k, c := range ln {
				r[k] = rune(c)
			}
		} else {
			r = []rune(string(ln))
		}
		lines = append(lines, r)
		if i < 0 {
			break
		}
		b = b[i+1:]
	}
	return
}

func LoadDoc(path string) (*Doc, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, errors.New("is a directory")
	}
	if st.Size() > 30<<20 {
		return nil, errors.New("file too large (>30 MB)")
	}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	head := b
	if len(head) > 8192 {
		head = head[:8192]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, errors.New("binary file - cannot open")
	}
	d := NewDoc()
	d.Path = path
	d.Lines, d.CRLF, d.BOM, d.Latin1 = decodeLines(b)
	d.Lang = langForFile(path)
	d.detectIndent()
	return d, nil
}

func (d *Doc) detectIndent() {
	tabs, spaces := 0, 0
	min := 0
	for i, l := range d.Lines {
		if i > 400 {
			break
		}
		if len(l) > 1 && l[0] == '\t' {
			tabs++
		} else if len(l) > 2 && l[0] == ' ' && l[1] == ' ' {
			spaces++
			n := 0
			for n < len(l) && l[n] == ' ' {
				n++
			}
			if n < len(l) && (min == 0 || n < min) {
				min = n
			}
		}
	}
	if tabs > spaces {
		d.UseTabs = true
		d.TabSize = 4
	} else if min == 2 || min == 4 || min == 8 {
		d.TabSize = min
	}
	if d.Lang != nil && (d.Lang.Name == "Go" || d.Lang.Name == "Makefile") && tabs >= spaces {
		d.UseTabs = true
	}
}

func (d *Doc) Bytes() []byte {
	var buf bytes.Buffer
	if d.BOM {
		buf.Write([]byte{0xEF, 0xBB, 0xBF})
	}
	nl := "\n"
	if d.CRLF {
		nl = "\r\n"
	}
	for i, l := range d.Lines {
		if i > 0 {
			buf.WriteString(nl)
		}
		if d.Latin1 {
			for _, r := range l {
				if r < 256 {
					buf.WriteByte(byte(r))
				} else {
					buf.WriteByte('?')
				}
			}
		} else {
			buf.WriteString(string(l))
		}
	}
	return buf.Bytes()
}

func (d *Doc) Save(path string) error {
	if path == "" {
		path = d.Path
	}
	if err := ioutil.WriteFile(path, d.Bytes(), 0644); err != nil {
		return err
	}
	if d.Path != path {
		d.Path = path
		d.Lang = langForFile(path)
		d.hlValid = 0
	}
	d.Dirty = false
	return nil
}

// ------------------------------------------------------------ geometry

func (d *Doc) LineLen(l int) int { return len(d.Lines[l]) }

func (d *Doc) clamp(p Pos) Pos {
	if p.L < 0 {
		p.L = 0
	}
	if p.L >= len(d.Lines) {
		p.L = len(d.Lines) - 1
	}
	if p.C < 0 {
		p.C = 0
	}
	if p.C > len(d.Lines[p.L]) {
		p.C = len(d.Lines[p.L])
	}
	return p
}

func (d *Doc) VisCol(l, c int) int {
	v := 0
	row := d.Lines[l]
	if c > len(row) {
		c = len(row)
	}
	for i := 0; i < c; i++ {
		if row[i] == '\t' {
			v += d.TabSize - v%d.TabSize
		} else {
			v++
		}
	}
	return v
}

func (d *Doc) ColAtVis(l, vis int) int {
	v := 0
	row := d.Lines[l]
	for i, r := range row {
		w := 1
		if r == '\t' {
			w = d.TabSize - v%d.TabSize
		}
		if vis < v+w {
			if vis-v >= (w+1)/2 && w > 1 {
				return i + 1
			}
			return i
		}
		v += w
	}
	return len(row)
}

func (d *Doc) Sel() (Pos, Pos, bool) {
	if d.Cur == d.Anchor {
		return d.Cur, d.Cur, false
	}
	if d.Cur.Less(d.Anchor) {
		return d.Cur, d.Anchor, true
	}
	return d.Anchor, d.Cur, true
}

func (d *Doc) Range(a, b Pos) []rune {
	if a.L == b.L {
		return append([]rune(nil), d.Lines[a.L][a.C:b.C]...)
	}
	var out []rune
	out = append(out, d.Lines[a.L][a.C:]...)
	for l := a.L + 1; l < b.L; l++ {
		out = append(out, '\n')
		out = append(out, d.Lines[l]...)
	}
	out = append(out, '\n')
	out = append(out, d.Lines[b.L][:b.C]...)
	return out
}

func (d *Doc) SelText() string {
	a, b, ok := d.Sel()
	if !ok {
		return ""
	}
	return string(d.Range(a, b))
}

func (d *Doc) moved() { d.lastKind = "" }

// ------------------------------------------------------------ edit core

func (d *Doc) snapshot() snap {
	return snap{append([][]rune(nil), d.Lines...), d.Cur, d.Anchor}
}

func (d *Doc) begin(kind string) {
	now := time.Now()
	co := (kind == "type" || kind == "del") && d.lastKind == kind && now.Sub(d.lastEdit) < 2*time.Second
	if !co {
		d.undo = append(d.undo, d.snapshot())
		max := 2000000 / (len(d.Lines) + 1)
		if max < 10 {
			max = 10
		}
		if max > 300 {
			max = 300
		}
		if len(d.undo) > max {
			d.undo = d.undo[len(d.undo)-max:]
		}
	}
	d.redo = nil
	d.lastKind, d.lastEdit = kind, now
}

func (d *Doc) touch(from int) {
	d.Dirty = true
	d.Version++
	if from < d.hlValid {
		d.hlValid = from
	}
}

func splitRunes(t []rune) [][]rune {
	var parts [][]rune
	start := 0
	for i, r := range t {
		if r == '\n' {
			parts = append(parts, append([]rune(nil), t[start:i]...))
			start = i + 1
		}
	}
	parts = append(parts, append([]rune(nil), t[start:]...))
	return parts
}

func (d *Doc) replace(a, b Pos, text []rune) Pos {
	parts := splitRunes(text)
	first := d.Lines[a.L][:a.C]
	last := d.Lines[b.L][b.C:]
	n := len(parts)
	var end Pos
	if n == 1 && a.L == b.L {
		row := make([]rune, 0, len(first)+len(parts[0])+len(last))
		row = append(append(append(row, first...), parts[0]...), last...)
		d.Lines[a.L] = row
		end = Pos{a.L, a.C + len(parts[0])}
		d.touch(a.L)
		return end
	}
	rows := make([][]rune, 0, n)
	r0 := make([]rune, 0, len(first)+len(parts[0]))
	r0 = append(append(r0, first...), parts[0]...)
	rows = append(rows, r0)
	for i := 1; i < n-1; i++ {
		rows = append(rows, parts[i])
	}
	if n > 1 {
		rl := make([]rune, 0, len(parts[n-1])+len(last))
		rl = append(append(rl, parts[n-1]...), last...)
		rows = append(rows, rl)
		end = Pos{a.L + n - 1, len(parts[n-1])}
	} else {
		rows[0] = append(rows[0], last...)
		end = Pos{a.L, a.C + len(parts[0])}
	}
	nl := make([][]rune, 0, len(d.Lines)-(b.L-a.L)-1+len(rows))
	nl = append(nl, d.Lines[:a.L]...)
	nl = append(nl, rows...)
	nl = append(nl, d.Lines[b.L+1:]...)
	d.Lines = nl
	d.touch(a.L)
	return end
}

func (d *Doc) setCur(p Pos, extend bool) {
	d.Cur = d.clamp(p)
	if !extend {
		d.Anchor = d.Cur
	}
	d.moved()
}

func (d *Doc) deleteSel() bool {
	a, b, ok := d.Sel()
	if !ok {
		return false
	}
	p := d.replace(a, b, nil)
	_ = p
	d.Cur, d.Anchor = a, a
	return true
}

func (d *Doc) InsertText(t string) {
	if d.ReadOnly || t == "" {
		return
	}
	d.begin("paste")
	d.deleteSel()
	end := d.replace(d.Cur, d.Cur, []rune(replaceAll(replaceAll(t, "\r\n", "\n"), "\r", "\n")))
	d.Cur, d.Anchor = end, end
	d.WantVis = -1
	d.moved()
}

var pairs = map[rune]rune{'(': ')', '[': ']', '{': '}', '"': '"', '\'': '\'', '`': '`'}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (d *Doc) TypeRune(r rune) {
	if d.ReadOnly {
		return
	}
	if a, b, ok := d.Sel(); ok {
		if cl, isOpen := pairs[r]; isOpen {
			d.begin("paste")
			txt := d.Range(a, b)
			nt := append(append([]rune{r}, txt...), cl)
			end := d.replace(a, b, nt)
			d.Anchor = Pos{a.L, a.C + 1}
			if a.L == b.L {
				d.Cur = Pos{end.L, end.C - 1}
			} else {
				d.Cur = Pos{end.L, end.C - 1}
			}
			d.moved()
			return
		}
	}
	d.begin("type")
	d.deleteSel()
	line := d.Lines[d.Cur.L]
	c := d.Cur.C
	var next, prev rune
	if c < len(line) {
		next = line[c]
	}
	if c > 0 {
		prev = line[c-1]
	}
	ins := []rune{r}
	back := 0
	if (r == ')' || r == ']' || r == '}' || r == '"' || r == '\'' || r == '`') && next == r {
		d.Cur.C++
		d.Anchor = d.Cur
		d.WantVis = -1
		return
	}
	if cl, ok := pairs[r]; ok {
		okc := false
		if r == '"' || r == '\'' || r == '`' {
			okc = !isWordRune(prev) && prev != r && (next == 0 || unicode.IsSpace(next) || strings.ContainsRune(")]},;:.", next))
		} else {
			okc = next == 0 || unicode.IsSpace(next) || strings.ContainsRune(")]},;", next)
		}
		if okc {
			ins = []rune{r, cl}
			back = 1
		}
	}
	end := d.replace(d.Cur, d.Cur, ins)
	end.C -= back
	d.Cur, d.Anchor = end, end
	d.WantVis = -1
}

func (d *Doc) IndentUnit() string {
	if d.UseTabs {
		return "\t"
	}
	return strings.Repeat(" ", d.TabSize)
}

func leadingWS(l []rune) int {
	n := 0
	for n < len(l) && (l[n] == ' ' || l[n] == '\t') {
		n++
	}
	return n
}

func (d *Doc) Newline(above, below bool) {
	if above || below {
		d.InsertLine(above)
		return
	}
	d.newlineAuto()
}

func decodeBytes(b []byte, d *Doc) {
	d.Lines, d.CRLF, d.BOM, d.Latin1 = decodeLines(b)
}

func (d *Doc) shiftLines(out bool) { d.IndentLines(out) }
func (d *Doc) SelectWordAt(p Pos)  { d.SelectWord(p) }
func (d *Doc) HL(l int) []Color    { return d.HighlightLine(l) }

func concat(parts ...[]rune) []rune {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	o := make([]rune, 0, n)
	for _, p := range parts {
		o = append(o, p...)
	}
	return o
}

func (d *Doc) newlineAuto() {
	if d.ReadOnly {
		return
	}
	d.begin("enter")
	d.deleteSel()
	line := d.Lines[d.Cur.L]
	c := d.Cur.C
	lw := leadingWS(line)
	if lw > c {
		lw = c
	}
	indent := string(line[:lw])
	before := strings.TrimRight(string(line[:c]), " \t")
	var last rune
	if before != "" {
		last, _ = utf8.DecodeLastRuneInString(before)
	}
	var next rune
	if c < len(line) {
		next = line[c]
	}
	extra := ""
	if last == '{' || last == '(' || last == '[' || (last == ':' && d.Lang != nil && d.Lang.ColonIndent) {
		extra = d.IndentUnit()
	}
	if (last == '{' && next == '}') || (last == '(' && next == ')') || (last == '[' && next == ']') {
		txt := []rune("\n" + indent + extra + "\n" + indent)
		end := d.replace(d.Cur, d.Cur, txt)
		p := Pos{end.L - 1, len([]rune(indent + extra))}
		d.Cur, d.Anchor = p, p
	} else {
		txt := []rune("\n" + indent + extra)
		end := d.replace(d.Cur, d.Cur, txt)
		d.Cur, d.Anchor = end, end
	}
	d.WantVis = -1
	d.moved()
}

func (d *Doc) Backspace(word bool) {
	if d.ReadOnly {
		return
	}
	if _, _, ok := d.Sel(); ok {
		d.begin("del")
		d.deleteSel()
		return
	}
	p := d.Cur
	if p.L == 0 && p.C == 0 {
		return
	}
	d.begin("del")
	line := d.Lines[p.L]
	var from Pos
	switch {
	case p.C == 0:
		from = Pos{p.L - 1, len(d.Lines[p.L-1])}
	case word:
		from = Pos{p.L, d.wordLeft(p).C}
	default:
		from = Pos{p.L, p.C - 1}
		if cl, ok := pairs[line[p.C-1]]; ok && p.C < len(line) && line[p.C] == cl {
			d.replace(Pos{p.L, p.C - 1}, Pos{p.L, p.C + 1}, nil)
			d.Cur = Pos{p.L, p.C - 1}
			d.Anchor = d.Cur
			d.WantVis = -1
			return
		}
		if !d.UseTabs && line[p.C-1] == ' ' && leadingWS(line) >= p.C && allSpaces(line[:p.C]) {
			stop := (p.C - 1) / d.TabSize * d.TabSize
			from.C = stop
		}
	}
	d.replace(from, p, nil)
	d.Cur, d.Anchor = from, from
	d.WantVis = -1
}

func allSpaces(r []rune) bool {
	for _, c := range r {
		if c != ' ' {
			return false
		}
	}
	return true
}

func (d *Doc) Delete(word bool) {
	if d.ReadOnly {
		return
	}
	if _, _, ok := d.Sel(); ok {
		d.begin("del")
		d.deleteSel()
		return
	}
	p := d.Cur
	var to Pos
	switch {
	case p.C >= len(d.Lines[p.L]):
		if p.L+1 >= len(d.Lines) {
			return
		}
		to = Pos{p.L + 1, 0}
	case word:
		to = d.wordRight(p)
	default:
		to = Pos{p.L, p.C + 1}
	}
	d.begin("del")
	d.replace(p, to, nil)
	d.WantVis = -1
}

func (d *Doc) Tab(shift bool) {
	if d.ReadOnly {
		return
	}
	a, b, ok := d.Sel()
	if ok && a.L != b.L || shift {
		d.IndentLines(shift)
		return
	}
	d.begin("paste")
	d.deleteSel()
	var ins string
	if d.UseTabs {
		ins = "\t"
	} else {
		v := d.VisCol(d.Cur.L, d.Cur.C)
		ins = strings.Repeat(" ", d.TabSize-v%d.TabSize)
	}
	end := d.replace(d.Cur, d.Cur, []rune(ins))
	d.Cur, d.Anchor = end, end
	d.WantVis = -1
}

func (d *Doc) lineRange() (int, int) {
	a, b, ok := d.Sel()
	if ok && b.C == 0 && b.L > a.L {
		return a.L, b.L - 1
	}
	return a.L, b.L
}

func (d *Doc) setRows(from, to int, rows [][]rune) {
	if len(rows) == to-from+1 {
		for i, r := range rows {
			d.Lines[from+i] = r
		}
	} else {
		nl := make([][]rune, 0, len(d.Lines)-(to-from+1)+len(rows))
		nl = append(nl, d.Lines[:from]...)
		nl = append(nl, rows...)
		nl = append(nl, d.Lines[to+1:]...)
		d.Lines = nl
	}
	d.touch(from)
}

func (d *Doc) shiftCols(from, to, delta func(l int) int) {}

func (d *Doc) IndentLines(out bool) {
	f, t := d.lineRange()
	d.begin("indent")
	unit := d.IndentUnit()
	rows := make([][]rune, 0, t-f+1)
	deltas := make([]int, 0, t-f+1)
	for l := f; l <= t; l++ {
		row := d.Lines[l]
		if !out {
			if len(row) == 0 {
				rows = append(rows, row)
				deltas = append(deltas, 0)
				continue
			}
			rows = append(rows, append([]rune(unit), row...))
			deltas = append(deltas, len([]rune(unit)))
		} else {
			n := 0
			if len(row) > 0 && row[0] == '\t' {
				n = 1
			} else {
				for n < len(row) && n < d.TabSize && row[n] == ' ' {
					n++
				}
			}
			rows = append(rows, append([]rune(nil), row[n:]...))
			deltas = append(deltas, -n)
		}
	}
	d.setRows(f, t, rows)
	adj := func(p Pos) Pos {
		if p.L >= f && p.L <= t {
			p.C += deltas[p.L-f]
			if p.C < 0 {
				p.C = 0
			}
		}
		return p
	}
	d.Cur, d.Anchor = d.clamp(adj(d.Cur)), d.clamp(adj(d.Anchor))
}

func (d *Doc) ToggleComment() bool {
	if d.Lang == nil || len(d.Lang.LineC) == 0 {
		return false
	}
	pre := d.Lang.LineC[0]
	f, t := d.lineRange()
	allC, any := true, false
	minInd := 1 << 30
	for l := f; l <= t; l++ {
		row := d.Lines[l]
		w := leadingWS(row)
		if w == len(row) {
			continue
		}
		any = true
		if w < minInd {
			minInd = w
		}
		if !strings.HasPrefix(string(row[w:]), pre) {
			allC = false
		}
	}
	if !any {
		return true
	}
	d.begin("comment")
	rows := make([][]rune, 0, t-f+1)
	deltas := make([]int, 0, t-f+1)
	pr := []rune(pre)
	for l := f; l <= t; l++ {
		row := d.Lines[l]
		w := leadingWS(row)
		if w == len(row) {
			rows = append(rows, row)
			deltas = append(deltas, 0)
			continue
		}
		if allC {
			n := len(pr)
			if w+n < len(row) && row[w+n] == ' ' {
				n++
			}
			nr := append(append([]rune(nil), row[:w]...), row[w+n:]...)
			rows = append(rows, nr)
			deltas = append(deltas, -n)
		} else {
			ins := append(append([]rune(nil), pr...), ' ')
			nr := append(append(append([]rune(nil), row[:minInd]...), ins...), row[minInd:]...)
			rows = append(rows, nr)
			deltas = append(deltas, len(ins))
		}
	}
	d.setRows(f, t, rows)
	adj := func(p Pos) Pos {
		if p.L >= f && p.L <= t {
			p.C += deltas[p.L-f]
		}
		return p
	}
	d.Cur, d.Anchor = d.clamp(adj(d.Cur)), d.clamp(adj(d.Anchor))
	return true
}

func (d *Doc) DuplicateLines() {
	f, t := d.lineRange()
	d.begin("dup")
	n := t - f + 1
	rows := make([][]rune, 0, 2*n)
	for l := f; l <= t; l++ {
		rows = append(rows, d.Lines[l])
	}
	for l := f; l <= t; l++ {
		rows = append(rows, append([]rune(nil), d.Lines[l]...))
	}
	d.setRows(f, t, rows)
	d.Cur.L += n
	d.Anchor.L += n
}

func (d *Doc) MoveLines(dir int) {
	f, t := d.lineRange()
	if (dir < 0 && f == 0) || (dir > 0 && t == len(d.Lines)-1) {
		return
	}
	d.begin("move")
	var rows [][]rune
	if dir < 0 {
		rows = append(rows, d.Lines[f:t+1]...)
		rows = append(rows, d.Lines[f-1])
		d.setRows(f-1, t, rows)
	} else {
		rows = append(rows, d.Lines[t+1])
		rows = append(rows, d.Lines[f:t+1]...)
		d.setRows(f, t+1, rows)
	}
	d.Cur.L += dir
	d.Anchor.L += dir
}

func (d *Doc) DeleteLines() {
	f, t := d.lineRange()
	d.begin("delline")
	if f == 0 && t == len(d.Lines)-1 {
		d.setRows(0, t, [][]rune{{}})
	} else {
		d.setRows(f, t, nil)
	}
	l := f
	if l >= len(d.Lines) {
		l = len(d.Lines) - 1
	}
	d.Cur, d.Anchor = Pos{l, 0}, Pos{l, 0}
	d.moved()
}

func (d *Doc) InsertLine(above bool) {
	d.begin("newline")
	l := d.Cur.L
	indent := append([]rune(nil), d.Lines[l][:leadingWS(d.Lines[l])]...)
	at := l + 1
	if above {
		at = l
	}
	nl := make([][]rune, 0, len(d.Lines)+1)
	nl = append(nl, d.Lines[:at]...)
	nl = append(nl, indent)
	nl = append(nl, d.Lines[at:]...)
	d.Lines = nl
	d.touch(at)
	d.Cur = Pos{at, len(indent)}
	d.Anchor = d.Cur
	d.moved()
}

func (d *Doc) Undo() bool {
	if len(d.undo) == 0 {
		return false
	}
	d.redo = append(d.redo, d.snapshot())
	s := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	d.Lines, d.Cur, d.Anchor = s.lines, s.cur, s.anchor
	d.touch(0)
	d.moved()
	return true
}

func (d *Doc) Redo() bool {
	if len(d.redo) == 0 {
		return false
	}
	d.undo = append(d.undo, d.snapshot())
	s := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	d.Lines, d.Cur, d.Anchor = s.lines, s.cur, s.anchor
	d.touch(0)
	d.moved()
	return true
}

// ------------------------------------------------------------ movement

func cls(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case isWordRune(r):
		return 1
	}
	return 2
}

func (d *Doc) wordLeft(p Pos) Pos {
	if p.C == 0 {
		if p.L > 0 {
			return Pos{p.L - 1, len(d.Lines[p.L-1])}
		}
		return p
	}
	row := d.Lines[p.L]
	c := p.C
	for c > 0 && cls(row[c-1]) == 0 {
		c--
	}
	if c > 0 {
		k := cls(row[c-1])
		for c > 0 && cls(row[c-1]) == k {
			c--
		}
	}
	return Pos{p.L, c}
}

func (d *Doc) wordRight(p Pos) Pos {
	row := d.Lines[p.L]
	if p.C >= len(row) {
		if p.L+1 < len(d.Lines) {
			return Pos{p.L + 1, 0}
		}
		return p
	}
	c := p.C
	k := cls(row[c])
	for c < len(row) && cls(row[c]) == k {
		c++
	}
	for c < len(row) && cls(row[c]) == 0 {
		c++
	}
	return Pos{p.L, c}
}

func (d *Doc) Move(key string, sel bool, page int) {
	p := d.Cur
	keepWant := false
	switch key {
	case "left":
		if _, _, ok := d.Sel(); ok && !sel {
			a, _, _ := d.Sel()
			p = a
		} else if p.C > 0 {
			p.C--
		} else if p.L > 0 {
			p = Pos{p.L - 1, len(d.Lines[p.L-1])}
		}
	case "right":
		if _, b, ok := d.Sel(); ok && !sel {
			p = b
		} else if p.C < len(d.Lines[p.L]) {
			p.C++
		} else if p.L+1 < len(d.Lines) {
			p = Pos{p.L + 1, 0}
		}
	case "up", "down", "pgup", "pgdn":
		n := 1
		if key == "pgup" || key == "pgdn" {
			n = page
		}
		if key == "up" || key == "pgup" {
			n = -n
		}
		if d.WantVis < 0 {
			d.WantVis = d.VisCol(p.L, p.C)
		}
		nl := p.L + n
		if nl < 0 {
			p = Pos{0, 0}
		} else if nl >= len(d.Lines) {
			p = Pos{len(d.Lines) - 1, len(d.Lines[len(d.Lines)-1])}
		} else {
			p = Pos{nl, d.ColAtVis(nl, d.WantVis)}
		}
		keepWant = true
	case "home":
		w := leadingWS(d.Lines[p.L])
		if p.C == w {
			p.C = 0
		} else {
			p.C = w
		}
	case "end":
		p.C = len(d.Lines[p.L])
	case "wordleft":
		p = d.wordLeft(p)
	case "wordright":
		p = d.wordRight(p)
	case "docstart":
		p = Pos{0, 0}
	case "docend":
		p = Pos{len(d.Lines) - 1, len(d.Lines[len(d.Lines)-1])}
	}
	if !keepWant {
		d.WantVis = -1
	}
	d.Cur = d.clamp(p)
	if !sel {
		d.Anchor = d.Cur
	}
	d.moved()
}

func (d *Doc) SelectAll() {
	d.Anchor = Pos{0, 0}
	d.Cur = Pos{len(d.Lines) - 1, len(d.Lines[len(d.Lines)-1])}
	d.moved()
}

func (d *Doc) SelectLine() {
	l := d.Cur.L
	d.Anchor = Pos{l, 0}
	if l+1 < len(d.Lines) {
		d.Cur = Pos{l + 1, 0}
	} else {
		d.Cur = Pos{l, len(d.Lines[l])}
	}
	d.moved()
}

func (d *Doc) SelectWord(p Pos) {
	p = d.clamp(p)
	row := d.Lines[p.L]
	if len(row) == 0 {
		d.Cur, d.Anchor = p, p
		return
	}
	c := p.C
	if c >= len(row) {
		c = len(row) - 1
	}
	k := cls(row[c])
	s, e := c, c+1
	for s > 0 && cls(row[s-1]) == k {
		s--
	}
	for e < len(row) && cls(row[e]) == k {
		e++
	}
	d.Anchor, d.Cur = Pos{p.L, s}, Pos{p.L, e}
	d.moved()
}

func (d *Doc) Goto(line, col int) {
	d.Cur = d.clamp(Pos{line, col})
	d.Anchor = d.Cur
	d.WantVis = -1
	d.moved()
}

// ------------------------------------------------------------ search

type Match struct{ L, C, N int }

type FindOpts struct {
	Case, Word, Regex bool
}

func lowerRunes(r []rune) []rune {
	o := make([]rune, len(r))
	for i, c := range r {
		o[i] = unicode.ToLower(c)
	}
	return o
}

func (d *Doc) FindAll(q string, cs, word, re bool) ([]Match, error) {
	o := FindOpts{cs, word, re}
	if q == "" {
		return nil, nil
	}
	var out []Match
	if o.Regex {
		pat := q
		if o.Word {
			pat = `\b(?:` + pat + `)\b`
		}
		if !o.Case {
			pat = "(?i)" + pat
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, err
		}
		for l, row := range d.Lines {
			s := string(row)
			for _, m := range re.FindAllStringIndex(s, -1) {
				if m[1] == m[0] {
					continue
				}
				c := utf8.RuneCountInString(s[:m[0]])
				out = append(out, Match{l, c, utf8.RuneCountInString(s[m[0]:m[1]])})
				if len(out) > 20000 {
					return out, nil
				}
			}
		}
		return out, nil
	}
	qr := []rune(q)
	if !o.Case {
		qr = lowerRunes(qr)
	}
	for l, row := range d.Lines {
		if len(row) < len(qr) {
			continue
		}
		h := row
		if !o.Case {
			h = lowerRunes(row)
		}
		for i := 0; i+len(qr) <= len(h); i++ {
			if h[i] != qr[0] {
				continue
			}
			ok := true
			for k := 1; k < len(qr); k++ {
				if h[i+k] != qr[k] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			if o.Word {
				if (i > 0 && isWordRune(h[i-1])) || (i+len(qr) < len(h) && isWordRune(h[i+len(qr)])) {
					continue
				}
			}
			out = append(out, Match{l, i, len(qr)})
			i += len(qr) - 1
			if len(out) > 20000 {
				return out, nil
			}
		}
	}
	return out, nil
}

// ------------------------------------------------------------ highlighting cache

func (d *Doc) HighlightLine(l int) []Color {
	if d.Lang == nil {
		d.Lang = langByName("Plain Text")
	}
	if len(d.hlStates) < len(d.Lines)+1 {
		ns := make([]hlState, len(d.Lines)+1)
		copy(ns, d.hlStates)
		d.hlStates = ns
	}
	if d.hlValid > len(d.Lines) {
		d.hlValid = len(d.Lines)
	}
	if d.hlValid == 0 {
		d.hlStates[0] = hlState{}
	}
	for d.hlValid < l {
		row := d.Lines[d.hlValid]
		if cap(d.hlBuf) < len(row) {
			d.hlBuf = make([]Color, len(row)+64)
		}
		d.hlStates[d.hlValid+1] = d.Lang.scan(row, d.hlStates[d.hlValid], d.hlBuf[:len(row)])
		d.hlValid++
	}
	row := d.Lines[l]
	out := make([]Color, len(row))
	d.Lang.scan(row, d.hlStates[l], out)
	return out
}
