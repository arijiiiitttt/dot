package src

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

// Run starts the editor with the given command-line arguments and returns the exit code.
func Run(osArgs []string) (code int) {
	forceVT, forceLegacy := false, false
	var targets []string
	for _, arg := range osArgs {
		switch strings.ToLower(arg) {
		case "--vt":
			forceVT = true
		case "--legacy":
			forceLegacy = true
		case "-v", "--version":
			fmt.Println("Dot " + version)
			return
		case "-h", "--help", "/?":
			fmt.Println("Dot " + version + " - terminal code editor\n\nUsage:  .dot [file|folder ...]\n\n  --legacy   force classic console renderer (Windows 7/8/10 cmd)\n  --vt       force ANSI/true-colour renderer (Windows Terminal)")
			return
		default:
			targets = append(targets, arg)
		}
	}
	cwd, _ := os.Getwd()
	root := cwd
	var files []string
	for _, t := range targets {
		st, err := os.Stat(t)
		if err == nil && st.IsDir() {
			root, _ = filepath.Abs(t)
		} else {
			files = append(files, t)
		}
	}

	be := NewBackend(forceVT, forceLegacy)
	if err := be.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "Dot must be started from a real console window (cmd or PowerShell):", err)
		return 1
	}
	closed := false
	closeBE := func() {
		if !closed {
			closed = true
			be.Close()
		}
	}
	defer func() {
		if r := recover(); r != nil {
			closeBE()
			fmt.Fprintf(os.Stderr, "Dot crashed: %v\n%s\n", r, debug.Stack())
			code = 2
		}
	}()
	defer closeBE()

	if u, ok := be.(interface{ Unicode() bool }); ok {
		uni = u.Unicode()
	}
	detectIconFont()
	w, h := be.Size()
	a := NewApp(be, root)
	a.scr = NewScreen(w, h)
	a.loadSettings()
	a.setTheme(themeIdx)
	defer a.saveSettings()
	for _, f := range files {
		a.openFile(f, 0)
	}
	a.render()

	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for !a.quit {
		select {
		case ev := <-be.Events():
			a.handle(ev)
			// drain a burst of events before painting
		drain:
			for i := 0; i < 64 && !a.quit; i++ {
				select {
				case e2 := <-be.Events():
					a.handle(e2)
				default:
					break drain
				}
			}
		case m := <-a.msgs:
			a.onProcMsg(m)
		case <-tick.C:
			if a.toast != "" && time.Now().After(a.toastUntil) {
				a.toast = ""
			}
		}
		if !a.quit {
			a.render()
		}
	}
	a.killAll()
	return 0
}