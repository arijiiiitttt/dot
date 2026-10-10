package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type PLine struct {
	T string
	C Color
}

type Pane struct {
	Name   string
	Lines  []PLine
	Scroll int
	cr     bool
}

func NewPane(name string) *Pane { return &Pane{Name: name, Lines: []PLine{{}}} }

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07]*\x07")

func (p *Pane) Add(chunk string) {
	chunk = ansiRe.ReplaceAllString(chunk, "")
	for _, r := range chunk {
		switch r {
		case '\n':
			p.Lines = append(p.Lines, PLine{C: cFg})
			p.cr = false
			if len(p.Lines) > 5000 {
				p.Lines = p.Lines[len(p.Lines)-4000:]
			}
		case '\r':
			p.cr = true
		default:
			last := &p.Lines[len(p.Lines)-1]
			if p.cr {
				last.T = ""
				p.cr = false
			}
			if r == '\t' {
				last.T += "    "
			} else if r >= 32 || r == 0x1b {
				last.T += string(r)
			}
		}
	}
}

// Println appends a complete coloured line (starting a fresh line first if needed).
func (p *Pane) Println(t string, c Color) {
	last := &p.Lines[len(p.Lines)-1]
	if last.T != "" {
		p.Lines = append(p.Lines, PLine{C: cFg})
		last = &p.Lines[len(p.Lines)-1]
	}
	last.T, last.C = t, c
	p.Lines = append(p.Lines, PLine{C: cFg})
}

func (p *Pane) Clear() { p.Lines, p.Scroll = []PLine{{}}, 0 }

// ---- processes -------------------------------------------------------------------------

type step struct {
	name  string
	args  []string
	line  string // run through the shell instead
	shell string // override shell
}

type Proc struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	killed bool
	pane   *Pane
	gen    int
}

type procMsg struct {
	proc *Proc
	text string
	done bool
	code int
}

func (a *App) running() bool { return a.proc != nil }

func (a *App) killProc() {
	p := a.proc
	if p == nil {
		return
	}
	p.mu.Lock()
	p.killed = true
	if p.cmd != nil && p.cmd.Process != nil {
		killTree(p.cmd)
	}
	p.mu.Unlock()
}

func decodeOut(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c)
	}
	return string(rs)
}

func (a *App) runSteps(pane *Pane, steps []step, cwd string) {
	if a.proc != nil {
		a.killProc()
	}
	proc := &Proc{pane: pane}
	a.proc = proc
	msgs := a.msgs
	shell := a.shell
	go func() {
		code := 0
		for _, st := range steps {
			var cmd *exec.Cmd
			if st.line != "" {
				sh := shell
				if st.shell != "" {
					sh = st.shell
				}
				cmd = shellCommand(sh, st.line)
			} else {
				cmd = exec.Command(st.name, st.args...)
			}
			hideWindow(cmd)
			cmd.Dir = cwd
			cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "PYTHONIOENCODING=utf-8", "NO_COLOR=1", "TERM=dumb")
			pr, pw, err := os.Pipe()
			if err != nil {
				msgs <- procMsg{proc, err.Error() + "\n", false, 0}
				code = 1
				break
			}
			cmd.Stdout, cmd.Stderr = pw, pw
			stdin, _ := cmd.StdinPipe()
			if err := cmd.Start(); err != nil {
				pw.Close()
				pr.Close()
				msgs <- procMsg{proc, "Cannot start: " + err.Error() + "\n", false, 0}
				code = 1
				break
			}
			pw.Close()
			proc.mu.Lock()
			proc.cmd, proc.stdin = cmd, stdin
			proc.mu.Unlock()
			buf := make([]byte, 8192)
			var carry []byte
			for {
				n, err := pr.Read(buf)
				if n > 0 {
					data := append(carry, buf[:n]...)
					carry = nil
					// keep an incomplete trailing UTF-8 sequence for the next read
					cut := len(data)
					for k := 1; k <= 3 && k <= len(data); k++ {
						if !utf8.FullRune(data[len(data)-k:]) {
							cut = len(data) - k
							break
						}
					}
					carry = append([]byte{}, data[cut:]...)
					msgs <- procMsg{proc, decodeOut(data[:cut]), false, 0}
				}
				if err != nil {
					break
				}
			}
			pr.Close()
			err = cmd.Wait()
			code = 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				code = 1
			}
			proc.mu.Lock()
			k := proc.killed
			proc.mu.Unlock()
			if code != 0 || k {
				if k {
					code = -1
				}
				break
			}
		}
		msgs <- procMsg{proc, "", true, code}
	}()
}

func (a *App) onProcMsg(m procMsg) {
	p := m.proc
	if m.done {
		if a.proc == p {
			a.proc = nil
		}
		if a.focus == fPanel && a.panelTab == 0 {
			a.focus = fEditor
		}
		if m.code == 0 {
			p.pane.Println("[Process finished successfully]", cType)
		} else if m.code == -1 {
			p.pane.Println("[Process stopped]", cFunc)
		} else {
			p.pane.Println("[Process exited with code "+strconv.Itoa(m.code)+"]", cRed)
		}
		return
	}
	p.pane.Add(m.text)
	if p.pane.Scroll > 0 {
		p.pane.Scroll += strings.Count(m.text, "\n")
	}
}

// ---- running the current file -----------------------------------------------------------

func firstExe(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func (a *App) stepsFor(path string) ([]step, string) {
	ext := strings.ToLower(filepath.Ext(path))
	need := func(label string, names ...string) (string, string) {
		if e := firstExe(names...); e != "" {
			return e, ""
		}
		return "", label + " was not found. Install it and make sure it is in your PATH."
	}
	win := runtime.GOOS == "windows"
	switch ext {
	case ".py", ".pyw":
		e, m := need("Python", "python", "py", "python3")
		return []step{{name: e, args: []string{path}}}, m
	case ".js", ".mjs", ".cjs":
		e, m := need("Node.js", "node")
		return []step{{name: e, args: []string{path}}}, m
	case ".ts":
		e, m := need("ts-node / tsx", "tsx", "ts-node")
		return []step{{name: e, args: []string{path}}}, m
	case ".go":
		e, m := need("Go", "go")
		return []step{{name: e, args: []string{"run", path}}}, m
	case ".lua":
		e, m := need("Lua", "lua", "lua5.4", "lua5.3", "luajit")
		return []step{{name: e, args: []string{path}}}, m
	case ".rb":
		e, m := need("Ruby", "ruby")
		return []step{{name: e, args: []string{path}}}, m
	case ".php":
		e, m := need("PHP", "php")
		return []step{{name: e, args: []string{path}}}, m
	case ".pl":
		e, m := need("Perl", "perl")
		return []step{{name: e, args: []string{path}}}, m
	case ".sh", ".bash":
		e, m := need("bash", "bash", "sh")
		return []step{{name: e, args: []string{path}}}, m
	case ".bat", ".cmd":
		return []step{{line: "\"" + path + "\"", shell: "cmd"}}, ""
	case ".ps1":
		e, m := need("PowerShell", "powershell", "pwsh")
		return []step{{name: e, args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path}}}, m
	case ".vbs":
		e, m := need("cscript", "cscript")
		return []step{{name: e, args: []string{"//nologo", path}}}, m
	case ".java":
		e, m := need("Java", "java")
		return []step{{name: e, args: []string{path}}}, m
	case ".html", ".htm":
		if win {
			return []step{{line: "start \"\" \"" + path + "\"", shell: "cmd"}}, ""
		}
		return nil, "Open the file in your browser to preview it."
	case ".c", ".cpp", ".cc", ".cxx":
		comp := "gcc"
		if ext != ".c" {
			comp = "g++"
		}
		e, m := need(comp+" (MinGW)", comp)
		out := filepath.Join(os.TempDir(), "dot_run_"+strconv.Itoa(os.Getpid()))
		if win {
			out += ".exe"
		}
		return []step{{name: e, args: []string{path, "-o", out}}, {name: out}}, m
	case ".rs":
		e, m := need("Rust (rustc)", "rustc")
		out := filepath.Join(os.TempDir(), "dot_run_"+strconv.Itoa(os.Getpid()))
		if win {
			out += ".exe"
		}
		return []step{{name: e, args: []string{path, "-o", out}}, {name: out}}, m
	}
	return nil, "Don't know how to run '" + ext + "' files. Use the terminal panel instead (Ctrl+`)."
}

func (a *App) runFile() {
	d := a.cur()
	if d == nil {
		a.err("Open a file first")
		return
	}
	if d.Path == "" {
		a.saveAs(func(a *App) { a.runFile() })
		return
	}
	if d.Dirty {
		if err := d.Save(""); err != nil {
			a.err(err.Error())
			return
		}
	}
	steps, msg := a.stepsFor(d.Path)
	a.showPanel(0)
	a.out.Clear()
	if msg != "" {
		a.out.Println(msg, cRed)
		return
	}
	a.out.Println("> "+a.rel(d.Path), cDim)
	a.runSteps(a.out, steps, filepath.Dir(d.Path))
	if a.running() {
		a.focus = fPanel // so typed input goes to the program (Esc returns to the editor)
	}
}

// ---- terminal ---------------------------------------------------------------------------

var driveRe = regexp.MustCompile(`^[A-Za-z]:$`)

func (a *App) termPrompt() string { return a.cwd + "> " }

func (a *App) termEnter() {
	line := a.termIn.String()
	a.termIn.Set("")
	a.term.Scroll = 0
	if a.running() && a.proc.pane == a.term {
		a.term.Add(line + "\n")
		a.sendStdin(line + "\n")
		return
	}
	a.term.Println(a.termPrompt()+line, cFg)
	t := strings.TrimSpace(line)
	if t == "" {
		return
	}
	if len(a.hist) == 0 || a.hist[len(a.hist)-1] != t {
		a.hist = append(a.hist, t)
	}
	a.hidx = len(a.hist)
	low := strings.ToLower(t)
	switch {
	case low == "cls" || low == "clear":
		a.term.Clear()
		return
	case low == "exit":
		a.panelVis = false
		a.focus = fEditor
		return
	case low == "cd" || strings.HasPrefix(low, "cd ") || strings.HasPrefix(low, "cd\\") || strings.HasPrefix(low, "cd/") || low == "cd..":
		arg := strings.TrimSpace(t[2:])
		if strings.HasPrefix(strings.ToLower(arg), "/d ") {
			arg = strings.TrimSpace(arg[3:])
		}
		arg = strings.Trim(arg, "\"")
		if arg == "" {
			a.term.Println(a.cwd, cFg)
			return
		}
		a.changeDir(arg)
		return
	case driveRe.MatchString(t):
		a.changeDir(t + string(filepath.Separator))
		return
	}
	if a.running() {
		a.killProc()
	}
	a.runSteps(a.term, []step{{line: t}}, a.cwd)
}

func (a *App) changeDir(arg string) {
	if arg == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			arg = h
		}
	}
	np := arg
	if !filepath.IsAbs(np) {
		np = filepath.Join(a.cwd, arg)
	}
	np = filepath.Clean(np)
	if st, err := os.Stat(np); err != nil || !st.IsDir() {
		a.term.Println("The system cannot find the path specified.", cRed)
		return
	}
	a.cwd = np
}

func (a *App) sendStdin(s string) {
	p := a.proc
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stdin != nil {
		io.WriteString(p.stdin, s)
	}
}
