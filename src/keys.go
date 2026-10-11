package src

import (
	"sort"
	"strings"
)

// Keyboard shortcut editor. User changes are stored in settings.ini as
//   key.ctrl+alt+x=edit.comment      (add a shortcut)
//   key.ctrl+s=none                  (remove a default shortcut)

var defaultKeymap []keyBind
var userKeys = map[string]string{}

func saveDefaultKeymap() { defaultKeymap = append([]keyBind(nil), keymap...) }

func rebuildKeyIndex() {
	for k := range keyIndex {
		delete(keyIndex, k)
	}
	for _, kb := range keymap {
		keyIndex[kb.Key] = kb.ID
	}
}

// applyUserKeys rebuilds keymap from the defaults plus the user's changes.
func applyUserKeys() {
	if defaultKeymap == nil {
		saveDefaultKeymap()
	}
	keymap = nil
	var extra []keyBind
	for _, kb := range defaultKeymap {
		if v, ok := userKeys[kb.Key]; ok {
			if v != "none" && cmdByID[v] != nil {
				extra = append(extra, keyBind{kb.Key, v})
			}
			continue
		}
		keymap = append(keymap, kb)
	}
	var ks []string
	for k := range userKeys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		v := userKeys[k]
		if v == "none" || cmdByID[v] == nil {
			continue
		}
		dup := false
		for _, e := range extra {
			if e.Key == k {
				dup = true
			}
		}
		if !dup {
			extra = append(extra, keyBind{k, v})
		}
	}
	keymap = append(keymap, extra...)
	rebuildKeyIndex()
}

// keysFor returns every key currently bound to a command.
func keysFor(id string) []string {
	var out []string
	for _, kb := range keymap {
		if kb.ID == id {
			out = append(out, prettyKey(kb.Key))
		}
	}
	return out
}

func validBindKey(k string) bool {
	if k == "" {
		return false
	}
	if strings.HasPrefix(k, "ctrl+") || strings.HasPrefix(k, "alt+") {
		return true
	}
	if k[0] == 'f' && len(k) >= 2 && k[1] >= '0' && k[1] <= '9' { // f1..f12, with or without shift
		return true
	}
	return strings.Contains(k, "+f") && strings.HasPrefix(k, "shift+")
}

func bindKey(key, id string) {
	userKeys[key] = id
	applyUserKeys()
}

func unbindCommand(id string) {
	for _, kb := range append([]keyBind(nil), keymap...) {
		if kb.ID != id {
			continue
		}
		isDefault := false
		for _, d := range defaultKeymap {
			if d.Key == kb.Key && d.ID == id {
				isDefault = true
			}
		}
		if isDefault {
			userKeys[kb.Key] = "none"
		} else {
			delete(userKeys, kb.Key)
		}
	}
	applyUserKeys()
}

func resetKeys() {
	userKeys = map[string]string{}
	applyUserKeys()
}

// ---- UI ------------------------------------------------------------------------------

func (a *App) openKeybindings(query string) {
	pad := func(k string) string { return k + strings.Repeat(" ", imax(1, 26-len([]rune(k)))) }
	var p *Palette
	items := []PItem{{Label: pad("") + "Reset ALL shortcuts to default", Run: func(a *App) {
		resetKeys()
		a.saveSettings()
		a.say("All keyboard shortcuts reset")
		a.openKeybindings("")
	}}}
	for i := range cmds {
		c := &cmds[i]
		ks := strings.Join(keysFor(c.ID), ", ")
		items = append(items, PItem{Label: pad(ks) + c.Title, Run: func(a *App) { a.captureKey(c, p.in.String()) }})
	}
	p = &Palette{fixed: items, hint: "Keyboard Shortcuts: Enter on a command, then press the new keys"}
	p.in.Set(query)
	p.refresh(a)
	a.overlay = p
}

type KeyCapture struct {
	cmd   *Cmd
	query string
	msg   string
}

func (a *App) captureKey(c *Cmd, q string) {
	a.overlay = &KeyCapture{cmd: c, query: q}
}

func (k *KeyCapture) Key(a *App, ev Event) {
	switch ev.Key {
	case "esc":
		a.openKeybindings(k.query)
		return
	case "backspace", "delete":
		unbindCommand(k.cmd.ID)
		a.saveSettings()
		a.say("Removed the shortcuts of: " + k.cmd.Title)
		a.openKeybindings(k.query)
		return
	}
	if !validBindKey(ev.Key) {
		k.msg = "Use Ctrl+..., Alt+... or an F key (not " + prettyKey(ev.Key) + ")"
		return
	}
	if old := keyIndex[ev.Key]; old != "" && old != k.cmd.ID {
		if oc := cmdByID[old]; oc != nil {
			a.say(prettyKey(ev.Key) + " moved from: " + oc.Title)
		}
	}
	bindKey(ev.Key, k.cmd.ID)
	a.saveSettings()
	a.openKeybindings(k.query)
}

func (k *KeyCapture) Mouse(a *App, ev Event) {
	if ev.Action == "down" && ev.Btn == 1 {
		a.openKeybindings(k.query)
	}
}

func (k *KeyCapture) Draw(a *App) {
	w, h := imin(a.W-2, 60), 7
	x, y := (a.W-w)/2, (a.H-h)/2
	a.drawBox(x, y, w, h, "Keyboard Shortcut", cAccent, cSide)
	s := a.scr
	s.PutN(x+2, y+1, w-4, k.cmd.Title, cFunc, cSide)
	cur := strings.Join(keysFor(k.cmd.ID), ", ")
	if cur == "" {
		cur = "(none)"
	}
	s.PutN(x+2, y+2, w-4, "Now: "+cur, cDim, cSide)
	s.PutN(x+2, y+4, w-4, "Press the new key combination...", cFg, cSide)
	hint := "Esc cancel   Backspace remove shortcuts"
	if k.msg != "" {
		hint, k.msg = k.msg, ""
		s.PutN(x+2, y+5, w-4, hint, cRed, cSide)
		return
	}
	s.PutN(x+2, y+5, w-4, hint, cDim, cSide)
}
