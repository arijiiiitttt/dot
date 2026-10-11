package src

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const version = "1.0"

type Focus int

const (
	fEditor Focus = iota
	fSide
	fPanel
	fFind
)

type Hit struct {
	R  Rect
	Fn func()
}

type FindState struct {
	open, repl bool
	q, r       Input
	focusR     bool
	cs, word   bool
	re         bool
	matches    []Match
	cur        int
	key        string
	doc        *Doc
	ver        int
	err        string
}

type Layout struct {
	actW, sideX, sideW  int
	edX, edW            int
	tabY, crumbY, bodyY int
	bodyH               int
	panelY, panelH      int
	statusY             int
}

type App struct {
	extraSettings string // alias.* lines of settings.ini, kept when saving
	be            Backend
	scr           *Screen
	W, H          int
	docs          []*Doc
	act           int
	root          string
	focus         Focus
	overlay       Overlay
	lay           Layout

	sideVis, panelVis bool
	sideView          int // 0 explorer 1 search
	sideW, panelH     int
	panelTab          int // 0 output 1 terminal
	exp               *Explorer
	srch              *Search
	find              *FindState

	out, term *Pane
	termIn    Input
	hist      []string
	hidx      int
	cwd       string
	shell     string
	proc      *Proc
	msgs      chan procMsg

	toast      string
	toastErr   bool
	toastUntil time.Time

	cx, cy   int
	showCur  bool
	hits     []Hit
	tabFirst int

	clip                string
	clipLine            bool
	untitled            int
	dragging            bool
	dragPanel, dragSide bool
	panelMax            bool
	panelPrev           int
	lastHdrClick        time.Time
	quit                bool
}

func NewApp(be Backend, root string) *App {
	a := &App{be: be, root: root, sideVis: true, panelVis: false, sideW: 12, panelH: 10}
	a.exp = NewExplorer(root)
	a.srch = &Search{inputFoc: true}
	a.find = &FindState{}
	a.out = NewPane("OUTPUT")
	a.term = NewPane("TERMINAL")
	a.cwd = root
	a.shell = defaultShell()
	a.msgs = make(chan procMsg, 256)
	return a
}

func (a *App) cur() *Doc {
	if a.act >= 0 && a.act < len(a.docs) {
		return a.docs[a.act]
	}
	return nil
}

func (a *App) say(s string) {
	a.toast, a.toastErr, a.toastUntil = s, false, time.Now().Add(4*time.Second)
}
func (a *App) err(s string) {
	a.toast, a.toastErr, a.toastUntil = s, true, time.Now().Add(6*time.Second)
}

func (a *App) clipGet() string {
	if t, ok := a.be.GetClip(); ok {
		return t
	}
	return a.clip
}

func (a *App) clipSet(s string) {
	a.clip = s
	a.be.SetClip(s)
}

// ---- documents --------------------------------------------------------------------------

func (a *App) addDoc(d *Doc) {
	a.docs = append(a.docs, d)
	a.act = len(a.docs) - 1
	a.focus = fEditor
}

func (a *App) newDoc() {
	a.untitled++
	d := NewDoc()
	d.Untitled = a.untitled
	a.addDoc(d)
}

// openAsText opens a file in the editor even if it is a picture.
func (a *App) openAsText(path string) { a.openFileX(path, 0, true) }

func (a *App) openFile(path string, line int) { a.openFileX(path, line, false) }

func (a *App) openFileX(path string, line int, forceText bool) {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		a.openFolder(path)
		return
	}
	if !forceText && isImagePath(path) {
		if _, err := os.Stat(path); err == nil {
			a.openImage(path)
			return
		}
	}
	for i, d := range a.docs {
		if d.Path == path {
			a.act = i
			a.focus = fEditor
			if line > 0 {
				a.gotoLine(line)
			}
			return
		}
	}
	d, err := LoadDoc(path)
	if err != nil {
		if os.IsNotExist(err) { // open as a new file that will be created on save
			d = NewDoc()
			d.Path = path
			d.Lang = langForFile(path)
		} else {
			a.err("Cannot open " + filepath.Base(path) + ": " + err.Error())
			return
		}
	}
	a.addDoc(d)
	a.exp.Reveal(path)
	if line > 0 {
		a.gotoLine(line)
	}
}

func (a *App) openFolder(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		a.err("Folder not found: " + path)
		return
	}
	a.root, a.cwd = abs, abs
	a.exp = NewExplorer(abs)
	a.sideVis, a.sideView = true, 0
	a.say("Opened folder " + abs)
}

func (a *App) removeDoc(i int) {
	a.docs = append(a.docs[:i], a.docs[i+1:]...)
	if a.act >= len(a.docs) {
		a.act = len(a.docs) - 1
	} else if i < a.act {
		a.act--
	}
	if a.act < 0 {
		a.act = 0
	}
	a.find.matches, a.find.key = nil, ""
}

func (a *App) closeDoc(i int) {
	d := a.docs[i]
	if !d.Dirty || d.Title != "" {
		a.removeDoc(i)
		return
	}
	a.confirm("Do you want to save the changes you made to "+d.Name()+"?\nYour changes will be lost if you don't save them.",
		[]string{"Save", "Don't Save", "Cancel"}, func(a *App, b int) {
			switch b {
			case 0:
				a.saveDoc(d, func(a *App) {
					for k, x := range a.docs {
						if x == d {
							a.removeDoc(k)
						}
					}
				})
			case 1:
				for k, x := range a.docs {
					if x == d {
						a.removeDoc(k)
					}
				}
			}
		})
}

func (a *App) saveDoc(d *Doc, done func(a *App)) {
	if d.Path == "" {
		a.saveAsDoc(d, done)
		return
	}
	if err := d.Save(""); err != nil {
		a.err("Save failed: " + err.Error())
		return
	}
	a.say("Saved " + d.Name())
	a.exp.Refresh()
	if done != nil {
		done(a)
	}
}

func (a *App) saveAs(done func(a *App)) {
	if d := a.cur(); d != nil {
		a.saveAsDoc(d, done)
	}
}

func (a *App) saveAsDoc(d *Doc, done func(a *App)) {
	init := d.Path
	if init == "" {
		init = filepath.Join(a.root, "untitled.txt")
	}
	a.prompt("Save as (file path)", init, func(a *App, p string) {
		if !filepath.IsAbs(p) {
			p = filepath.Join(a.root, p)
		}
		write := func(a *App) {
			os.MkdirAll(filepath.Dir(p), 0755)
			if err := d.Save(p); err != nil {
				a.err("Save failed: " + err.Error())
				return
			}
			a.say("Saved " + d.Name())
			a.exp.Refresh()
			a.exp.Reveal(p)
			if done != nil {
				done(a)
			}
		}
		if _, err := os.Stat(p); err == nil && p != d.Path {
			a.confirm("'"+filepath.Base(p)+"' already exists. Replace it?", []string{"Replace", "Cancel"}, func(a *App, i int) {
				if i == 0 {
					write(a)
				}
			})
			return
		}
		write(a)
	})
}

func (a *App) requestQuit() {
	var dirty []*Doc
	for _, d := range a.docs {
		if d.Dirty && d.Title == "" {
			dirty = append(dirty, d)
		}
	}
	if len(dirty) == 0 {
		a.quit = true
		return
	}
	a.confirm("There are "+itoa(len(dirty))+" file(s) with unsaved changes.\nSave them before exiting?", []string{"Save All", "Don't Save", "Cancel"}, func(a *App, i int) {
		switch i {
		case 0:
			for _, d := range dirty {
				if d.Path == "" {
					a.err("Save '" + d.Name() + "' first (Ctrl+S), then exit again")
					a.act = indexOf(a.docs, d)
					return
				}
				if err := d.Save(""); err != nil {
					a.err("Save failed: " + err.Error())
					return
				}
			}
			a.quit = true
		case 1:
			a.quit = true
		}
	})
}

func indexOf(ds []*Doc, d *Doc) int {
	for i, x := range ds {
		if x == d {
			return i
		}
	}
	return 0
}

func (a *App) gotoLine(n int) {
	d := a.cur()
	if d == nil || n < 1 {
		return
	}
	if n > len(d.Lines) {
		n = len(d.Lines)
	}
	d.setCur(Pos{n - 1, 0}, false)
	a.focus = fEditor
	a.fixScroll()
	// centre the line
	d.ScrollY = imax(0, n-1-a.lay.bodyH/2)
}

// ---- layout & scrolling ---------------------------------------------------------------------

func (a *App) setPanelH(h int) {
	a.panelH = imax(4, imin(a.H-8, h))
	a.panelMax = false
}

func (a *App) togglePanelMax() {
	if a.panelMax {
		a.panelH, a.panelMax = a.panelPrev, false
		return
	}
	a.panelPrev = a.panelH
	a.panelH, a.panelMax = a.H-8, true
}

func (a *App) computeLayout() {
	l := &a.lay
	W, H := a.W, a.H
	l.statusY = H - 1
	mainH := H - 2
	l.actW = 3
	if W < 40 {
		l.actW = 0
	}
	l.sideW = 0
	if a.sideVis && W >= 50 {
		l.sideW = imin(a.sideW, W/2)
	}
	l.sideX = l.actW
	l.edX = l.actW + l.sideW
	l.edW = W - l.edX
	ph := 0
	if a.panelVis {
		ph = imin(a.panelH, mainH-5)
		if ph < 3 {
			ph = 0
		}
	}
	l.panelH = ph
	l.tabY, l.crumbY, l.bodyY = 1, 2, 3
	l.bodyH = imax(1, mainH-2-ph)
	l.panelY = l.bodyY + l.bodyH
}

func digits(n int) int {
	c := 1
	for n >= 10 {
		n /= 10
		c++
	}
	return c
}

func gutterW(d *Doc) int { return imax(3, digits(len(d.Lines))) + 3 }

func (a *App) fixScroll() {
	d := a.cur()
	if d == nil {
		return
	}
	a.computeLayout()
	bh := a.lay.bodyH
	tw := imax(4, a.lay.edW-gutterW(d)-1)
	if d.Cur.L < d.ScrollY {
		d.ScrollY = d.Cur.L
	}
	if d.Cur.L >= d.ScrollY+bh {
		d.ScrollY = d.Cur.L - bh + 1
	}
	vc := d.VisCol(d.Cur.L, d.Cur.C)
	if vc < d.ScrollX {
		d.ScrollX = imax(0, vc-6)
	}
	if vc >= d.ScrollX+tw-1 {
		d.ScrollX = vc - tw + 3
	}
	if d.ScrollY > len(d.Lines)-1 {
		d.ScrollY = len(d.Lines) - 1
	}
}

func (a *App) showPanel(tab int) {
	a.panelVis = true
	a.panelTab = tab
	if tab == 1 {
		a.focus = fPanel
	}
}

func (a *App) onResize() {
	w, h := a.be.Size()
	a.W, a.H = w, h
	a.scr.Resize(w, h)
}

// ---- commands -------------------------------------------------------------------------------

type Cmd struct {
	ID, Title string
	Run       func(a *App)
}

var cmds []Cmd
var cmdByID = map[string]*Cmd{}

type keyBind struct{ Key, ID string }

var keymap = []keyBind{
	{"ctrl+n", "file.new"}, {"ctrl+o", "file.open"}, {"ctrl+p", "file.quickopen"}, {"ctrl+s", "file.save"},
	{"ctrl+shift+s", "file.saveas"}, {"ctrl+alt+s", "file.saveall"}, {"ctrl+w", "file.close"}, {"ctrl+q", "file.exit"},
	{"ctrl+z", "edit.undo"}, {"ctrl+y", "edit.redo"}, {"ctrl+shift+z", "edit.redo"},
	{"ctrl+x", "edit.cut"}, {"ctrl+c", "edit.copy"}, {"ctrl+v", "edit.paste"}, {"shift+insert", "edit.paste"}, {"ctrl+a", "edit.selectall"},
	{"ctrl+f", "edit.find"}, {"ctrl+h", "edit.replace"}, {"f3", "edit.findnext"}, {"shift+f3", "edit.findprev"},
	{"ctrl+/", "edit.comment"}, {"ctrl+l", "sel.line"}, {"alt+shift+down", "sel.dup"}, {"alt+up", "sel.up"}, {"alt+down", "sel.down"},
	{"ctrl+shift+k", "sel.delete"}, {"ctrl+]", "sel.indent"}, {"ctrl+[", "sel.outdent"},
	{"ctrl+enter", "sel.below"}, {"ctrl+shift+enter", "sel.above"},
	{"ctrl+b", "view.sidebar"}, {"ctrl+j", "view.panel"}, {"ctrl+shift+e", "view.explorer"}, {"ctrl+shift+f", "view.search"},
	{"ctrl+shift+p", "help.leader"}, {"f1", "help.leader"}, {"ctrl+`", "view.terminal"}, {"ctrl+shift+u", "view.output"},
	{"ctrl+g", "go.line"}, {"ctrl+pgdn", "go.next"}, {"ctrl+pgup", "go.prev"}, {"alt+.", "go.next"}, {"alt+,", "go.prev"},
	{"f5", "run.file"}, {"ctrl+f5", "run.file"}, {"shift+f5", "run.stop"},
	{"f2", "explorer.rename"}, {"f10", "view.menu"},
	{"ctrl+alt+up", "view.panelbigger"}, {"ctrl+alt+down", "view.panelsmaller"}, {"ctrl+shift+m", "view.panelmax"},
	{"ctrl+space", "help.leader"}, {"ctrl+,", "view.settings"}, {"f4", "help.leader"}, {"ctrl+k", "view.keybindings"}, {"ctrl+alt+t", "view.theme"},
}

var keyIndex = map[string]string{}

func prettyKey(k string) string {
	parts := strings.Split(k, "+")
	for i, p := range parts {
		switch p {
		case "pgdn":
			p = "PgDn"
		case "pgup":
			p = "PgUp"
		case "esc":
			p = "Esc"
		case "":
			p = "+"
		default:
			if len(p) > 0 {
				p = strings.ToUpper(p[:1]) + p[1:]
			}
		}
		parts[i] = p
	}
	return strings.Join(parts, "+")
}

func keyFor(id string) string {
	if id == "" {
		return ""
	}
	for _, kb := range keymap {
		if kb.ID == id {
			return prettyKey(kb.Key)
		}
	}
	return ""
}

func (a *App) needDoc() *Doc {
	d := a.cur()
	if d == nil {
		a.err("No file is open")
	}
	return d
}

func init() {
	add := func(id, title string, f func(a *App)) { cmds = append(cmds, Cmd{id, title, f}) }
	add("file.new", "File: New File", func(a *App) { a.newDoc() })
	add("file.open", "File: Open File...", func(a *App) {
		a.prompt("Open file (path, relative to the folder or absolute)", a.root+string(filepath.Separator), func(a *App, p string) {
			if !filepath.IsAbs(p) {
				p = filepath.Join(a.root, p)
			}
			a.openFile(p, 0)
		})
	})
	add("file.openfolder", "File: Open Folder...", func(a *App) {
		a.prompt("Open folder (path)", a.root, func(a *App, p string) { a.openFolder(p) })
	})
	add("file.quickopen", "File: Go to File...", func(a *App) { a.openPalette("") })
	add("file.save", "File: Save", func(a *App) {
		if d := a.needDoc(); d != nil {
			a.saveDoc(d, nil)
		}
	})
	add("file.saveas", "File: Save As...", func(a *App) {
		if d := a.needDoc(); d != nil {
			a.saveAsDoc(d, nil)
		}
	})
	add("file.saveall", "File: Save All", func(a *App) {
		n := 0
		for _, d := range a.docs {
			if d.Dirty && d.Path != "" && d.Title == "" {
				if err := d.Save(""); err != nil {
					a.err(err.Error())
					return
				}
				n++
			}
		}
		a.say("Saved " + itoa(n) + " file(s)")
	})
	add("file.close", "File: Close Editor", func(a *App) {
		if a.cur() != nil {
			a.closeDoc(a.act)
		}
	})
	add("file.closeall", "File: Close All Editors", func(a *App) {
		var dirty bool
		for _, d := range a.docs {
			dirty = dirty || (d.Dirty && d.Title == "")
		}
		closeAll := func(a *App) { a.docs, a.act = nil, 0 }
		if !dirty {
			closeAll(a)
			return
		}
		a.confirm("Some files have unsaved changes. Close them all anyway?", []string{"Close", "Cancel"}, func(a *App, i int) {
			if i == 0 {
				closeAll(a)
			}
		})
	})
	add("file.exit", "File: Exit", func(a *App) { a.requestQuit() })

	add("edit.undo", "Edit: Undo", func(a *App) {
		if d := a.needDoc(); d != nil && !d.Undo() {
			a.say("Nothing to undo")
		}
	})
	add("edit.redo", "Edit: Redo", func(a *App) {
		if d := a.needDoc(); d != nil && !d.Redo() {
			a.say("Nothing to redo")
		}
	})
	add("edit.copy", "Edit: Copy", func(a *App) {
		d := a.needDoc()
		if d == nil {
			return
		}
		if t := d.SelText(); t != "" {
			a.clipSet(t)
			a.clipLine = false
		} else {
			a.clipSet(string(d.Lines[d.Cur.L]) + "\n")
			a.clipLine = true
		}
	})
	add("edit.cut", "Edit: Cut", func(a *App) {
		d := a.needDoc()
		if d == nil || d.ReadOnly {
			return
		}
		if t := d.SelText(); t != "" {
			a.clipSet(t)
			a.clipLine = false
			d.begin("cut")
			d.deleteSel()
		} else {
			a.clipSet(string(d.Lines[d.Cur.L]) + "\n")
			a.clipLine = true
			d.DeleteLines()
		}
	})
	add("edit.paste", "Edit: Paste", func(a *App) {
		d := a.needDoc()
		if d == nil {
			return
		}
		t := a.clipGet()
		if t == "" {
			return
		}
		if a.clipLine && t == a.clip && d.Cur == d.Anchor {
			d.Cur = Pos{d.Cur.L, 0}
			d.Anchor = d.Cur
			d.InsertText(t)
			d.Cur = Pos{d.Cur.L, 0}
			d.Anchor = d.Cur
			return
		}
		d.InsertText(t)
	})
	add("edit.selectall", "Edit: Select All", func(a *App) {
		if d := a.needDoc(); d != nil {
			d.SelectAll()
		}
	})
	add("edit.find", "Edit: Find", func(a *App) { a.openFind(false) })
	add("edit.replace", "Edit: Replace", func(a *App) { a.openFind(true) })
	add("edit.findnext", "Edit: Find Next", func(a *App) { a.findStep(1) })
	add("edit.findprev", "Edit: Find Previous", func(a *App) { a.findStep(-1) })
	add("edit.comment", "Edit: Toggle Line Comment", func(a *App) {
		if d := a.needDoc(); d != nil && !d.ToggleComment() {
			a.err(d.Lang.Name + " has no line comment")
		}
	})
	add("sel.line", "Selection: Select Line", func(a *App) {
		if d := a.needDoc(); d != nil {
			d.SelectLine()
		}
	})
	add("sel.dup", "Selection: Duplicate Line", func(a *App) {
		if d := a.needDoc(); d != nil && !d.ReadOnly {
			d.DuplicateLines()
		}
	})
	add("sel.up", "Selection: Move Line Up", func(a *App) {
		if d := a.needDoc(); d != nil && !d.ReadOnly {
			d.MoveLines(-1)
		}
	})
	add("sel.down", "Selection: Move Line Down", func(a *App) {
		if d := a.needDoc(); d != nil && !d.ReadOnly {
			d.MoveLines(1)
		}
	})
	add("sel.delete", "Selection: Delete Line", func(a *App) {
		if d := a.needDoc(); d != nil && !d.ReadOnly {
			d.DeleteLines()
		}
	})
	add("sel.indent", "Selection: Indent Line", func(a *App) {
		if d := a.needDoc(); d != nil && !d.ReadOnly {
			d.shiftLines(false)
		}
	})
	add("sel.outdent", "Selection: Outdent Line", func(a *App) {
		if d := a.needDoc(); d != nil && !d.ReadOnly {
			d.shiftLines(true)
		}
	})
	add("sel.below", "Selection: Insert Line Below", func(a *App) {
		if d := a.needDoc(); d != nil {
			d.Newline(false, true)
		}
	})
	add("sel.above", "Selection: Insert Line Above", func(a *App) {
		if d := a.needDoc(); d != nil {
			d.Newline(true, false)
		}
	})

	add("view.palette", "View: Command Palette", func(a *App) { a.openPalette(">") })
	add("view.menu", "View: Open Menu Bar", func(a *App) { a.openMenu(0) })
	add("view.explorer", "View: Show Explorer", func(a *App) {
		a.sideVis, a.sideView, a.focus = true, 0, fSide
	})
	add("view.search", "View: Search in Files", func(a *App) {
		a.sideVis, a.sideView, a.focus = true, 1, fSide
		a.srch.inputFoc = true
		if d := a.cur(); d != nil {
			if t := d.SelText(); t != "" && !strings.Contains(t, "\n") {
				a.srch.in.Set(t)
			}
		}
	})
	add("view.sidebar", "View: Toggle Side Bar", func(a *App) {
		a.sideVis = !a.sideVis
		if !a.sideVis && a.focus == fSide {
			a.focus = fEditor
		}
	})
	add("view.panel", "View: Toggle Panel", func(a *App) {
		a.panelVis = !a.panelVis
		if !a.panelVis && a.focus == fPanel {
			a.focus = fEditor
		}
		if a.panelVis {
			a.focus = fPanel
		}
	})
	add("view.terminal", "View: Toggle Terminal", func(a *App) {
		if a.panelVis && a.panelTab == 1 {
			a.panelVis = false
			a.focus = fEditor
			return
		}
		a.showPanel(1)
	})
	add("view.output", "View: Show Output", func(a *App) { a.showPanel(0) })
	add("view.panelmax", "View: Maximize / Restore Panel", func(a *App) { a.panelVis = true; a.togglePanelMax() })
	add("view.icons", "View: Toggle File Icons (auto / on / off)", func(a *App) {
		iconMode = (iconMode + 1) % 3
		a.saveSettings()
		a.say("File icons: " + []string{"auto", "on", "off"}[iconMode])
	})
	add("view.settings", "Preferences: Open Settings", func(a *App) { a.openSettings() })
	add("view.theme", "Preferences: Color Theme...", func(a *App) { a.pickTheme() })
	add("view.keybindings", "Preferences: Keyboard Shortcuts...", func(a *App) { a.openKeybindings("") })
	add("view.image", "View: Open Image Viewer...", func(a *App) {
		if d := a.cur(); d != nil && d.Path != "" && isImagePath(d.Path) {
			a.openImage(d.Path)
			return
		}
		var items []PItem
		for _, f := range a.listFiles() {
			if isImagePath(f) {
				p := filepath.Join(a.root, f)
				items = append(items, PItem{Label: f, File: f, Run: func(a *App) { a.openImage(p) }})
			}
		}
		if len(items) == 0 {
			a.err("No PNG / JPEG / GIF images found in this folder")
			return
		}
		a.pick("Choose an image to view", items)
	})
	add("help.leader", "Help: Which Key (leader menu)", func(a *App) { a.openWhichKey() })
	add("view.panelbigger", "View: Increase Panel Size", func(a *App) { a.panelVis = true; a.setPanelH(a.panelH + 3) })
	add("view.panelsmaller", "View: Decrease Panel Size", func(a *App) { a.setPanelH(a.panelH - 3) })
	add("view.sidewider", "View: Widen Side Bar", func(a *App) { a.sideW = imin(a.sideW+4, 60) })
	add("view.sidenarrower", "View: Narrow Side Bar", func(a *App) { a.sideW = imax(14, a.sideW-4) })
	add("view.windowsize", "View: Change Window Size...", func(a *App) {
		mk := func(w, h int) PItem {
			return PItem{Label: itoa(w) + " x " + itoa(h), Run: func(a *App) {
				if a.be.Resize(w, h) {
					a.onResize()
				} else {
					a.err("This terminal cannot be resized by Dot - resize the window by hand")
				}
			}}
		}
		a.pick("Choose a window size (columns x rows)", []PItem{mk(80, 25), mk(100, 30), mk(120, 35), mk(140, 40), mk(160, 45), mk(200, 60)})
	})
	add("go.line", "Go: Go to Line...", func(a *App) { a.openPalette(":") })
	add("go.next", "Go: Next Editor", func(a *App) {
		if len(a.docs) > 0 {
			a.act = (a.act + 1) % len(a.docs)
		}
	})
	add("go.prev", "Go: Previous Editor", func(a *App) {
		if len(a.docs) > 0 {
			a.act = (a.act + len(a.docs) - 1) % len(a.docs)
		}
	})
	add("run.file", "Run: Run File", func(a *App) { a.runFile() })
	add("run.stop", "Run: Stop", func(a *App) {
		if a.running() {
			a.killProc()
		}
	})
	add("term.clear", "Terminal: Clear", func(a *App) {
		a.term.Clear()
		a.out.Clear()
	})
	add("term.shell", "Terminal: Select Default Shell", func(a *App) {
		var items []PItem
		for _, s := range shellChoices() {
			s := s
			items = append(items, PItem{Label: s, Run: func(a *App) { a.shell = s; a.saveSettings(); a.say("Terminal shell: " + s) }})
		}
		a.pick("Select the shell used by the terminal panel", items)
	})
	add("explorer.newfile", "Explorer: New File...", func(a *App) { a.explorerNewFile() })
	add("explorer.newfolder", "Explorer: New Folder...", func(a *App) { a.explorerNewFolder() })
	add("explorer.rename", "Explorer: Rename...", func(a *App) {
		if a.focus == fSide && a.sideView == 0 {
			a.explorerRename()
		}
	})
	add("explorer.delete", "Explorer: Delete...", func(a *App) { a.explorerDelete() })
	add("explorer.refresh", "Explorer: Refresh", func(a *App) { a.exp.Refresh() })
	add("editor.spaces", "Editor: Indent Using Spaces", func(a *App) {
		if d := a.needDoc(); d != nil {
			d.UseTabs = false
		}
	})
	add("editor.tabs", "Editor: Indent Using Tabs", func(a *App) {
		if d := a.needDoc(); d != nil {
			d.UseTabs = true
		}
	})
	add("editor.tabsize", "Editor: Change Tab Size...", func(a *App) {
		d := a.needDoc()
		if d == nil {
			return
		}
		var items []PItem
		for _, n := range []int{2, 3, 4, 8} {
			n := n
			items = append(items, PItem{Label: itoa(n), Run: func(a *App) { d.TabSize = n }})
		}
		a.pick("Tab size", items)
	})
	add("editor.eol", "Editor: Change End of Line (CRLF / LF)", func(a *App) {
		if d := a.needDoc(); d != nil {
			d.CRLF = !d.CRLF
			d.Dirty = true
			a.say(map[bool]string{true: "Line endings: CRLF", false: "Line endings: LF"}[d.CRLF])
		}
	})
	add("editor.lang", "Editor: Change Language Mode...", func(a *App) {
		d := a.needDoc()
		if d == nil {
			return
		}
		var items []PItem
		for _, l := range langs {
			l := l
			items = append(items, PItem{Label: l.Name, Run: func(a *App) { d.Lang = l; d.hlValid = 0 }})
		}
		a.pick("Select language mode", items)
	})
	add("help.keys", "Help: Keyboard Shortcuts", func(a *App) { a.openPalette("?") })
	add("help.welcome", "Help: Welcome", func(a *App) { a.docs, a.act = nil, 0 })
	add("help.about", "Help: About Dot", func(a *App) {
		a.confirm("Dot "+version+" - terminal code editor\nRuns on Windows 7 / 8 / 10 / 11 (cmd, PowerShell, Terminal)\nNo install needed. Press F1 for all commands.", []string{"OK"}, func(a *App, i int) {})
	})
	for i := range cmds {
		cmdByID[cmds[i].ID] = &cmds[i]
	}
	for _, kb := range keymap {
		keyIndex[kb.Key] = kb.ID
	}
}

func (a *App) showShortcuts() {
	var sb strings.Builder
	sb.WriteString("DOT - KEYBOARD SHORTCUTS\n\n")
	seen := map[string]bool{}
	for _, c := range cmds {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		k := keyFor(c.ID)
		if k == "" {
			continue
		}
		sb.WriteString(k + strings.Repeat(" ", imax(1, 20-len(k))) + c.Title + "\n")
	}
	sb.WriteString("\nEditing\n")
	sb.WriteString("Arrows / Home / End / PgUp / PgDn   Move (add Shift to select)\n")
	sb.WriteString("Ctrl+Left / Ctrl+Right              Move by word\n")
	sb.WriteString("Ctrl+Home / Ctrl+End                Start / end of file\n")
	sb.WriteString("Ctrl+Backspace / Ctrl+Delete        Delete word\n")
	sb.WriteString("Tab / Shift+Tab                     Indent / outdent\n")
	sb.WriteString("Mouse: click, drag, double-click, wheel, right-click\n")
	sb.WriteString("Alt+F / E / S / V / G / R / T / H   Open menus\n")
	d := NewDoc()
	decodeBytes([]byte(sb.String()), d)
	d.Title = "Keyboard Shortcuts"
	d.ReadOnly = true
	d.Dirty = false
	a.addDoc(d)
}

// ---- find / replace -----------------------------------------------------------------------------

func (a *App) openFind(repl bool) {
	d := a.cur()
	if d == nil {
		return
	}
	f := a.find
	f.open = true
	f.repl = repl || f.repl && repl
	if repl {
		f.repl = true
	}
	if t := d.SelText(); t != "" && !strings.Contains(t, "\n") {
		f.q.Set(t)
	}
	f.focusR = repl && f.q.String() != ""
	a.focus = fFind
	a.findRefresh(true)
}

func (a *App) closeFind() {
	a.find.open = false
	a.focus = fEditor
}

func (a *App) findMatches() []Match {
	f := a.find
	d := a.cur()
	if !f.open || d == nil {
		return nil
	}
	key := f.q.String() + "|" + map[bool]string{true: "1", false: "0"}[f.cs] + map[bool]string{true: "1", false: "0"}[f.word] + map[bool]string{true: "1", false: "0"}[f.re]
	if f.doc != d || f.ver != d.Version || f.key != key {
		m, err := d.FindAll(f.q.String(), f.cs, f.word, f.re)
		f.matches, f.doc, f.ver, f.key = m, d, d.Version, key
		f.err = ""
		if err != nil {
			f.err = "bad regex"
		}
	}
	return f.matches
}

func (a *App) findRefresh(jump bool) {
	d := a.cur()
	if d == nil {
		return
	}
	a.findMatches()
	if jump && len(a.find.matches) > 0 {
		a.findFrom(d.Cur, true)
		a.find.cur = a.curMatchIndex()
	}
}

func (a *App) curMatchIndex() int {
	d := a.cur()
	if d == nil {
		return -1
	}
	s, e, ok := d.Sel()
	if !ok {
		return -1
	}
	for i, m := range a.find.matches {
		if m.L == s.L && m.C == s.C && m.L == e.L && m.C+m.N == e.C {
			return i
		}
	}
	return -1
}

// findFrom selects the first match at/after (fwd) or before (back) p.
func (a *App) findFrom(p Pos, fwd bool) {
	d := a.cur()
	ms := a.find.matches
	if d == nil || len(ms) == 0 {
		return
	}
	pick := -1
	if fwd {
		for i, m := range ms {
			if !(Pos{m.L, m.C}).Less(p) {
				pick = i
				break
			}
		}
		if pick < 0 {
			pick = 0
		}
	} else {
		for i := len(ms) - 1; i >= 0; i-- {
			m := ms[i]
			if !p.Less(Pos{m.L, m.C + m.N}) {
				pick = i
				break
			}
		}
		if pick < 0 {
			pick = len(ms) - 1
		}
	}
	m := ms[pick]
	d.Anchor, d.Cur = Pos{m.L, m.C}, Pos{m.L, m.C + m.N}
	d.moved()
	d.WantVis = -1
	a.find.cur = pick
	a.fixScroll()
	d.ScrollY = imax(0, imin(d.ScrollY, m.L))
	if m.L < d.ScrollY || m.L >= d.ScrollY+a.lay.bodyH {
		d.ScrollY = imax(0, m.L-a.lay.bodyH/2)
	}
}

func (a *App) findStep(dir int) {
	d := a.cur()
	if d == nil {
		return
	}
	if !a.find.open && a.find.q.String() == "" {
		a.openFind(false)
		return
	}
	prev := a.find.open
	a.find.open = true
	a.findMatches()
	a.find.open = prev
	if len(a.find.matches) == 0 {
		a.err("No results")
		return
	}
	s, e, ok := d.Sel()
	if dir > 0 {
		p := d.Cur
		if ok {
			p = e
		}
		a.findFrom(p, true)
	} else {
		p := d.Cur
		if ok {
			p = s
		}
		a.findFrom(p, false)
	}
}

func (a *App) replaceOne() {
	d := a.cur()
	f := a.find
	if d == nil || d.ReadOnly {
		return
	}
	a.findMatches()
	i := a.curMatchIndex()
	if i < 0 {
		a.findStep(1)
		return
	}
	m := f.matches[i]
	d.begin("replace")
	rep := a.expandRepl(d, m)
	end := d.replace(Pos{m.L, m.C}, Pos{m.L, m.C + m.N}, []rune(rep))
	d.Cur, d.Anchor = end, end
	a.findMatches()
	a.findFrom(end, true)
}

func (a *App) expandRepl(d *Doc, m Match) string {
	f := a.find
	r := f.r.String()
	if !f.re {
		return r
	}
	pat := f.q.String()
	if !f.cs {
		pat = "(?i)" + pat
	}
	rx, err := regexp.Compile(pat)
	if err != nil {
		return r
	}
	return rx.ReplaceAllString(string(d.Lines[m.L][m.C:m.C+m.N]), r)
}

func (a *App) replaceAll() {
	d := a.cur()
	if d == nil || d.ReadOnly {
		return
	}
	ms := a.findMatches()
	if len(ms) == 0 {
		a.err("No results")
		return
	}
	d.begin("replace")
	ms = append([]Match{}, ms...)
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		d.replace(Pos{m.L, m.C}, Pos{m.L, m.C + m.N}, []rune(a.expandRepl(d, m)))
	}
	d.Cur, d.Anchor = d.clamp(d.Cur), d.clamp(d.Anchor)
	a.say("Replaced " + itoa(len(ms)) + " occurrence(s)")
}

func (a *App) findKey(ev Event) bool {
	f := a.find
	in := &f.q
	if f.focusR {
		in = &f.r
	}
	switch ev.Key {
	case "esc":
		a.closeFind()
		return true
	case "enter":
		if f.focusR {
			a.replaceOne()
		} else {
			a.findStep(1)
			a.focus = fFind
		}
		return true
	case "shift+enter":
		a.findStep(-1)
		a.focus = fFind
		return true
	case "ctrl+enter":
		if f.repl {
			a.replaceAll()
			return true
		}
		return false
	case "tab", "shift+tab":
		if f.repl {
			f.focusR = !f.focusR
		}
		return true
	case "alt+c":
		f.cs = !f.cs
		a.findRefresh(true)
		return true
	case "alt+w":
		f.word = !f.word
		a.findRefresh(true)
		return true
	case "alt+r":
		f.re = !f.re
		a.findRefresh(true)
		return true
	case "up", "down", "pgup", "pgdn", "f3", "shift+f3":
		if ev.Key == "up" || ev.Key == "down" {
			a.findStep(map[string]int{"up": -1, "down": 1}[ev.Key])
			a.focus = fFind
			return true
		}
		return false
	}
	before := in.String()
	if in.Key(a, ev) {
		if !f.focusR && in.String() != before {
			a.findRefresh(true)
		}
		return true
	}
	return false
}

// ---- event handling ------------------------------------------------------------------------------

var menuMnemonics = map[string]int{"alt+f": 0, "alt+e": 1, "alt+s": 2, "alt+v": 3, "alt+g": 4, "alt+r": 5, "alt+t": 6, "alt+h": 7}

func (a *App) handle(ev Event) {
	switch ev.Kind {
	case evResize:
		a.onResize()
	case evKey:
		a.onKey(ev)
	case evMouse:
		a.onMouse(ev)
	}
}

func (a *App) onKey(ev Event) {
	if a.overlay != nil {
		a.overlay.Key(a, ev)
		return
	}
	if i, ok := menuMnemonics[ev.Key]; ok && !(a.focus == fFind) {
		a.openMenu(i)
		return
	}
	switch a.focus {
	case fFind:
		if a.find.open && a.findKey(ev) {
			return
		}
	case fPanel:
		if a.panelKey(ev) {
			return
		}
	case fSide:
		if a.sideKey(ev) {
			return
		}
	}
	if ev.Key != "" {
		if id := keyIndex[ev.Key]; id != "" {
			if c := cmdByID[id]; c != nil {
				c.Run(a)
				if a.focus == fEditor || a.focus == fFind {
					a.fixScroll()
				}
				return
			}
		}
	}
	if a.focus == fFind {
		a.focus = fEditor
	}
	if a.focus == fEditor {
		a.editorKey(ev)
	}
}

func (a *App) editorKey(ev Event) {
	d := a.cur()
	if d == nil {
		return
	}
	page := imax(1, a.lay.bodyH-1)
	if ev.Key == "" {
		if ev.Rune != 0 && !unicode.IsControl(ev.Rune) {
			d.TypeRune(ev.Rune)
		}
		a.fixScroll()
		return
	}
	k := ev.Key
	ext := false
	if strings.HasPrefix(k, "ctrl+shift+") {
		ext = true
		k = "ctrl+" + k[len("ctrl+shift+"):]
	} else if strings.HasPrefix(k, "shift+") && !strings.HasPrefix(k, "shift+alt") {
		ext = true
		k = k[len("shift+"):]
	}
	switch k {
	case "left", "right", "up", "down", "home", "end", "pgup", "pgdn":
		d.Move(k, ext, page)
	case "ctrl+left":
		d.Move("wordleft", ext, page)
	case "ctrl+right":
		d.Move("wordright", ext, page)
	case "ctrl+home":
		d.Move("docstart", ext, page)
	case "ctrl+end":
		d.Move("docend", ext, page)
	case "enter":
		d.Newline(false, false)
	case "backspace":
		d.Backspace(false)
	case "ctrl+backspace":
		d.Backspace(true)
	case "delete":
		d.Delete(false)
	case "ctrl+delete":
		d.Delete(true)
	case "tab":
		d.Tab(ev.Shift)
	case "esc":
		d.Anchor = d.Cur
	case "space":
		d.TypeRune(' ')
	}
	if ev.Key == "shift+tab" {
		d.Tab(true)
	}
	a.fixScroll()
}

func (a *App) sideKey(ev Event) bool {
	if ev.Key == "esc" {
		a.focus = fEditor
		return true
	}
	if a.sideView == 1 {
		return a.searchKey(ev)
	}
	e := a.exp
	n := e.Selected()
	page := imax(1, a.H-6)
	switch ev.Key {
	case "up":
		e.Move(-1)
	case "down":
		e.Move(1)
	case "pgup":
		e.Move(-page)
	case "pgdn":
		e.Move(page)
	case "home":
		e.Sel = 0
	case "end":
		e.Sel = len(e.rows) - 1
	case "left":
		if n != nil && n.Dir && n.Open {
			e.Toggle(n)
		} else if n != nil && n.Parent != nil {
			for i, r := range e.rows {
				if r == n.Parent {
					e.Sel = i
				}
			}
		}
	case "right":
		if n != nil && n.Dir && !n.Open {
			e.Toggle(n)
		} else if n != nil && n.Dir {
			e.Move(1)
		}
	case "enter":
		if n != nil {
			if n.Dir {
				e.Toggle(n)
			} else {
				a.openFile(n.Path, 0)
			}
		}
	case "delete":
		a.explorerDelete()
	case "f2":
		a.explorerRename()
	default:
		return false
	}
	return true
}

func (a *App) searchKey(ev Event) bool {
	s := a.srch
	if s.inputFoc {
		switch ev.Key {
		case "enter":
			a.runSearch()
			return true
		case "down":
			if len(s.rows) > 0 {
				s.inputFoc = false
			}
			return true
		case "alt+c":
			s.cs = !s.cs
			return true
		}
		return s.in.Key(a, ev)
	}
	move := func(d int) {
		i := s.sel + d
		for i >= 0 && i < len(s.rows) && s.rows[i].Line == 0 {
			i += d
		}
		if i >= 0 && i < len(s.rows) {
			s.sel = i
		} else if d < 0 {
			s.inputFoc = true
		}
	}
	switch ev.Key {
	case "up":
		move(-1)
	case "down":
		move(1)
	case "enter":
		if s.sel < len(s.rows) && s.rows[s.sel].Line > 0 {
			r := s.rows[s.sel]
			a.openFile(r.Path, r.Line)
			if d := a.cur(); d != nil {
				d.Anchor = Pos{r.Line - 1, r.Col}
				d.Cur = Pos{r.Line - 1, r.Col + len([]rune(s.in.String()))}
				d.Cur = d.clamp(d.Cur)
				d.Anchor = d.clamp(d.Anchor)
			}
		}
	case "alt+c":
		s.cs = !s.cs
	default:
		if ev.Key == "" && ev.Rune != 0 {
			s.inputFoc = true
			return s.in.Key(a, ev)
		}
		return false
	}
	return true
}

func (a *App) panelKey(ev Event) bool {
	pane := a.out
	if a.panelTab == 1 {
		pane = a.term
	}
	busy := a.running() && a.proc.pane == pane
	switch ev.Key {
	case "esc":
		a.focus = fEditor
		return true
	case "ctrl+c":
		if busy {
			a.killProc()
			return true
		}
		return false
	case "pgup":
		pane.Scroll += imax(1, a.lay.panelH-2)
		return true
	case "pgdn":
		pane.Scroll = imax(0, pane.Scroll-imax(1, a.lay.panelH-2))
		return true
	case "ctrl+l":
		pane.Clear()
		return true
	case "enter":
		if a.panelTab == 1 {
			a.termEnter()
		} else if busy {
			line := a.termIn.String()
			a.termIn.Set("")
			pane.Add(line + "\n")
			a.sendStdin(line + "\n")
		}
		return true
	case "up", "down":
		if a.panelTab == 1 && !busy && len(a.hist) > 0 {
			if ev.Key == "up" {
				a.hidx = imax(0, a.hidx-1)
			} else {
				a.hidx = imin(len(a.hist), a.hidx+1)
			}
			if a.hidx < len(a.hist) {
				a.termIn.Set(a.hist[a.hidx])
			} else {
				a.termIn.Set("")
			}
			return true
		}
		return false
	case "tab":
		return true
	}
	if a.panelTab == 0 && !busy {
		return false
	}
	if ev.Key == "" || ev.Key == "backspace" || ev.Key == "delete" || ev.Key == "left" || ev.Key == "right" ||
		ev.Key == "home" || ev.Key == "end" || ev.Key == "ctrl+left" || ev.Key == "ctrl+right" || ev.Key == "ctrl+backspace" ||
		ev.Key == "ctrl+v" || ev.Key == "shift+insert" || ev.Key == "ctrl+u" {
		return a.termIn.Key(a, ev)
	}
	return false
}

// ---- mouse ---------------------------------------------------------------------------------------

func (a *App) hitAt(x, y int) bool {
	for i := len(a.hits) - 1; i >= 0; i-- {
		if a.hits[i].R.In(x, y) {
			a.hits[i].Fn()
			return true
		}
	}
	return false
}

func (a *App) onMouse(ev Event) {
	if a.overlay != nil {
		a.overlay.Mouse(a, ev)
		return
	}
	a.computeLayout()
	l := &a.lay
	d := a.cur()
	inEditor := ev.X >= l.edX && ev.Y >= l.bodyY && ev.Y < l.bodyY+l.bodyH
	inSide := ev.X >= l.sideX && ev.X < l.sideX+l.sideW && ev.Y >= 1 && ev.Y < a.H-1
	inPanel := l.panelH > 0 && ev.X >= l.edX && ev.Y >= l.panelY && ev.Y < l.panelY+l.panelH
	switch ev.Action {
	case "wheelup", "wheeldown":
		dir := 3
		if ev.Action == "wheelup" {
			dir = -3
		}
		switch {
		case inEditor && d != nil:
			if ev.Shift {
				d.ScrollX = imax(0, d.ScrollX+dir*3)
			} else {
				d.ScrollY = imax(0, imin(len(d.Lines)-1, d.ScrollY+dir))
			}
		case inSide:
			if a.sideView == 0 {
				a.exp.Top = imax(0, imin(len(a.exp.rows)-1, a.exp.Top+dir))
			} else {
				a.srch.top = imax(0, imin(len(a.srch.rows)-1, a.srch.top+dir))
			}
		case inPanel:
			p := a.out
			if a.panelTab == 1 {
				p = a.term
			}
			p.Scroll = imax(0, p.Scroll-dir)
		}
	case "up":
		a.dragging, a.dragPanel, a.dragSide = false, false, false
	case "drag":
		if a.dragPanel {
			a.setPanelH(a.H - 1 - ev.Y)
			return
		}
		if a.dragSide {
			a.sideW = imax(14, imin(a.W/2, ev.X-l.sideX+1))
			return
		}
		if a.dragging && d != nil {
			a.editorPointer(d, ev, true)
		}
	case "down":
		if ev.Y == 0 {
			for i := range menuBar {
				if ev.X >= menuX(i) && ev.X < menuX(i)+len(menuBar[i].Title)+2 {
					a.openMenu(i)
					return
				}
			}
			return
		}
		if ev.Btn == 1 && a.hitAt(ev.X, ev.Y) {
			return
		}
		if ev.Btn == 1 && l.panelH > 0 && ev.X >= l.edX && ev.Y == l.panelY {
			a.focus = fPanel
			if ev.Double || time.Since(a.lastHdrClick) < 400*time.Millisecond {
				a.togglePanelMax()
				a.lastHdrClick = time.Time{}
				return
			}
			a.lastHdrClick = time.Now()
			a.dragPanel = true
			return
		}
		if ev.Btn == 1 && l.sideW > 0 && ev.X == l.edX && ev.Y >= 1 && ev.Y < a.H-1 && ev.Y < l.panelY+l.panelH {
			a.dragSide = true
			return
		}
		switch {
		case inSide:
			a.sideMouse(ev)
		case inEditor && d != nil:
			a.focus = fEditor
			if ev.Btn == 2 {
				a.editorPointer(d, ev, false)
				a.openPopup(ev.X, ev.Y, []MItem{{"Cut", "edit.cut", nil, false}, {"Copy", "edit.copy", nil, false}, {"Paste", "edit.paste", nil, false},
					{"", "", nil, true}, {"Select All", "edit.selectall", nil, false}, {"Toggle Line Comment", "edit.comment", nil, false},
					{"", "", nil, true}, {"Command Palette...", "view.palette", nil, false}})
				return
			}
			a.editorPointer(d, ev, ev.Shift)
			if ev.Double {
				d.SelectWordAt(d.Cur)
			}
			a.dragging = true
		case inPanel:
			a.focus = fPanel
		}
	}
}

func (a *App) editorPointer(d *Doc, ev Event, extend bool) {
	l := &a.lay
	gw := gutterW(d)
	row := ev.Y - l.bodyY
	if row < 0 {
		d.ScrollY = imax(0, d.ScrollY-1)
		row = 0
	}
	if row >= l.bodyH {
		d.ScrollY = imin(len(d.Lines)-1, d.ScrollY+1)
		row = l.bodyH - 1
	}
	ln := imin(len(d.Lines)-1, d.ScrollY+row)
	vis := ev.X - l.edX - gw + d.ScrollX
	if vis < 0 {
		vis = 0
	}
	col := d.ColAtVis(ln, vis)
	d.setCur(Pos{ln, col}, extend)
	d.WantVis = -1
	if ev.Action == "drag" {
		tw := l.edW - gw - 1
		if ev.X >= l.edX+gw+tw {
			d.ScrollX++
		} else if ev.X < l.edX+gw && d.ScrollX > 0 {
			d.ScrollX--
		}
	}
}

func (a *App) sideMouse(ev Event) {
	l := &a.lay
	if a.sideView == 0 {
		e := a.exp
		i := e.Top + (ev.Y - 2)
		if ev.Y < 2 || i < 0 || i >= len(e.rows) {
			a.focus = fSide
			return
		}
		e.Sel = i
		a.focus = fSide
		n := e.rows[i]
		if ev.Btn == 2 {
			a.explorerMenu(ev.X, ev.Y)
			return
		}
		if n.Dir {
			e.Toggle(n)
		} else {
			a.openFile(n.Path, 0)
		}
		return
	}
	s := a.srch
	a.focus = fSide
	if ev.Y == 2 {
		s.inputFoc = true
		return
	}
	i := s.top + (ev.Y - 4)
	if ev.Y >= 4 && i >= 0 && i < len(s.rows) {
		s.sel, s.inputFoc = i, false
		if s.rows[i].Line > 0 {
			r := s.rows[i]
			a.openFile(r.Path, r.Line)
			if d := a.cur(); d != nil {
				d.Anchor = d.clamp(Pos{r.Line - 1, r.Col})
				d.Cur = d.clamp(Pos{r.Line - 1, r.Col + len([]rune(s.in.String()))})
			}
		}
	}
	_ = l
}
