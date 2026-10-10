package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Node struct {
	Name, Path string
	Dir, Open  bool
	Loaded     bool
	Kids       []*Node
	Depth      int
	Parent     *Node
}

type Explorer struct {
	Root     *Node
	rows     []*Node
	Sel, Top int
}

func NewExplorer(root string) *Explorer {
	e := &Explorer{}
	e.Root = &Node{Name: filepath.Base(root), Path: root, Dir: true, Open: true}
	if e.Root.Name == "" || e.Root.Name == "." || e.Root.Name == string(filepath.Separator) {
		e.Root.Name = root
	}
	e.load(e.Root)
	e.flatten()
	return e
}

func (e *Explorer) load(n *Node) {
	ents, err := os.ReadDir(n.Path)
	n.Loaded = true
	old := map[string]*Node{}
	for _, k := range n.Kids {
		old[k.Name] = k
	}
	n.Kids = nil
	if err != nil {
		return
	}
	for _, en := range ents {
		if en.Name() == ".git" || en.Name() == "$RECYCLE.BIN" || en.Name() == "System Volume Information" {
			continue
		}
		k := &Node{Name: en.Name(), Path: filepath.Join(n.Path, en.Name()), Dir: en.IsDir(), Depth: n.Depth + 1, Parent: n}
		if o := old[k.Name]; o != nil && o.Dir == k.Dir {
			k.Open, k.Kids, k.Loaded = o.Open, o.Kids, o.Loaded
		}
		n.Kids = append(n.Kids, k)
	}
	sort.SliceStable(n.Kids, func(i, j int) bool {
		a, b := n.Kids[i], n.Kids[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

func (e *Explorer) Refresh() {
	var rec func(n *Node)
	rec = func(n *Node) {
		if n.Dir && n.Loaded {
			e.load(n)
			for _, k := range n.Kids {
				rec(k)
			}
		}
	}
	rec(e.Root)
	e.flatten()
}

func (e *Explorer) flatten() {
	sel := e.Selected()
	e.rows = e.rows[:0]
	var rec func(n *Node)
	rec = func(n *Node) {
		e.rows = append(e.rows, n)
		if n.Dir && n.Open {
			for _, k := range n.Kids {
				rec(k)
			}
		}
	}
	rec(e.Root)
	if sel != nil {
		for i, r := range e.rows {
			if r == sel {
				e.Sel = i
			}
		}
	}
	if e.Sel >= len(e.rows) {
		e.Sel = len(e.rows) - 1
	}
	if e.Sel < 0 {
		e.Sel = 0
	}
}

func (e *Explorer) Selected() *Node {
	if e.Sel >= 0 && e.Sel < len(e.rows) {
		return e.rows[e.Sel]
	}
	return nil
}

func (e *Explorer) Toggle(n *Node) {
	if !n.Dir {
		return
	}
	n.Open = !n.Open
	if n.Open && !n.Loaded {
		e.load(n)
	}
	e.flatten()
}

func (e *Explorer) Reveal(path string) {
	rel, err := filepath.Rel(e.Root.Path, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	n := e.Root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if !n.Loaded {
			e.load(n)
		}
		n.Open = true
		var next *Node
		for _, k := range n.Kids {
			if k.Name == part {
				next = k
			}
		}
		if next == nil {
			break
		}
		n = next
	}
	e.flatten()
	for i, r := range e.rows {
		if r == n {
			e.Sel = i
		}
	}
}

func (e *Explorer) Move(d int) {
	e.Sel = imax(0, imin(len(e.rows)-1, e.Sel+d))
}

// ---- explorer actions on the app ----------------------------------------------------

func (a *App) targetDir() string {
	n := a.exp.Selected()
	if n == nil {
		return a.root
	}
	if n.Dir {
		return n.Path
	}
	return filepath.Dir(n.Path)
}

func (a *App) explorerNewFile() {
	dir := a.targetDir()
	a.prompt("New file in "+a.rel(dir)+"  (Enter to create)", "", func(a *App, name string) {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			a.err("'" + name + "' already exists")
			return
		}
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := ioutil.WriteFile(p, nil, 0644); err != nil {
			a.err(err.Error())
			return
		}
		a.exp.Refresh()
		a.openFile(p, 0)
	})
}

func (a *App) explorerNewFolder() {
	dir := a.targetDir()
	a.prompt("New folder in "+a.rel(dir), "", func(a *App, name string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0755); err != nil {
			a.err(err.Error())
			return
		}
		a.exp.Refresh()
		a.exp.Reveal(p)
	})
}

func (a *App) explorerRename() {
	n := a.exp.Selected()
	if n == nil || n == a.exp.Root {
		return
	}
	a.prompt("Rename "+n.Name, n.Name, func(a *App, name string) {
		np := filepath.Join(filepath.Dir(n.Path), name)
		if err := os.Rename(n.Path, np); err != nil {
			a.err(err.Error())
			return
		}
		for _, d := range a.docs {
			if d.Path == n.Path {
				d.Path = np
				d.Lang = langForFile(np)
				d.hlValid = 0
			}
		}
		a.exp.Refresh()
		a.exp.Reveal(np)
	})
}

func (a *App) explorerDelete() {
	n := a.exp.Selected()
	if n == nil || n == a.exp.Root {
		return
	}
	a.confirm("Delete '"+n.Name+"'?\nThis cannot be undone.", []string{"Delete", "Cancel"}, func(a *App, i int) {
		if i != 0 {
			return
		}
		if err := os.RemoveAll(n.Path); err != nil {
			a.err(err.Error())
			return
		}
		for k := len(a.docs) - 1; k >= 0; k-- {
			if a.docs[k].Path == n.Path || strings.HasPrefix(a.docs[k].Path, n.Path+string(filepath.Separator)) {
				a.removeDoc(k)
			}
		}
		a.exp.Refresh()
		a.say("Deleted " + n.Name)
	})
}

func (a *App) explorerMenu(x, y int) {
	a.openPopup(x, y, []MItem{
		{"New File", "", func(a *App) { a.explorerNewFile() }, false},
		{"New Folder", "", func(a *App) { a.explorerNewFolder() }, false},
		{"", "", nil, true},
		{"Rename", "", func(a *App) { a.explorerRename() }, false},
		{"Delete", "", func(a *App) { a.explorerDelete() }, false},
		{"", "", nil, true},
		{"Copy Path", "", func(a *App) {
			if n := a.exp.Selected(); n != nil {
				a.clipSet(n.Path)
				a.say("Path copied")
			}
		}, false},
		{"Refresh", "", func(a *App) { a.exp.Refresh() }, false},
	})
}

// ---- search in files -------------------------------------------------------------------

type SRow struct {
	Path string
	Line int // 1-based; 0 for file headers
	Col  int
	Text string
	N    int
}

type Search struct {
	in       Input
	rows     []SRow
	sel, top int
	inputFoc bool
	cs       bool
	status   string
}

func (a *App) runSearch() {
	s := a.srch
	q := s.in.String()
	s.rows, s.sel, s.top = nil, 0, 0
	if q == "" {
		s.status = ""
		return
	}
	ql := q
	if !s.cs {
		ql = strings.ToLower(q)
	}
	skip := map[string]bool{".git": true, "node_modules": true, "__pycache__": true, ".vs": true, ".idea": true}
	start := time.Now()
	total, files := 0, 0
	stopped := false
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if stopped || depth > 10 {
			return
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if stopped {
				return
			}
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if !skip[e.Name()] {
					walk(p, depth+1)
				}
				continue
			}
			if time.Since(start) > 4*time.Second || total >= 2000 {
				stopped = true
				return
			}
			info, err := e.Info()
			if err != nil || info.Size() > 2<<20 {
				continue
			}
			b, err := ioutil.ReadFile(p)
			if err != nil {
				continue
			}
			n := imin(len(b), 4096)
			bin := false
			for i := 0; i < n; i++ {
				if b[i] == 0 {
					bin = true
					break
				}
			}
			if bin {
				continue
			}
			var hits []SRow
			for ln, line := range strings.Split(string(b), "\n") {
				line = strings.TrimRight(line, "\r")
				hay := line
				if !s.cs {
					hay = strings.ToLower(line)
				}
				if i := strings.Index(hay, ql); i >= 0 {
					t := strings.TrimSpace(line)
					if len(t) > 200 {
						t = t[:200]
					}
					hits = append(hits, SRow{Path: p, Line: ln + 1, Col: len([]rune(line[:i])), Text: t})
				}
			}
			if len(hits) > 0 {
				files++
				total += len(hits)
				s.rows = append(s.rows, SRow{Path: p, N: len(hits)})
				s.rows = append(s.rows, hits...)
			}
		}
	}
	walk(a.root, 0)
	s.status = itoa(total) + " results in " + itoa(files) + " files"
	if stopped {
		s.status += " (stopped early)"
	}
	if total == 0 {
		s.status = "No results found"
	}
	s.inputFoc = false
	for i, r := range s.rows {
		if r.Line > 0 {
			s.sel = i
			break
		}
	}
}
