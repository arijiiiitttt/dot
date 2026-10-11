package src

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Settings are saved to a small key=value file so they survive a restart:
//   Windows: %APPDATA%\Dot\settings.ini      Linux/macOS: ~/.config/dot/settings.ini

func settingsPath() string {
	if d := os.Getenv("APPDATA"); d != "" {
		return filepath.Join(d, "Dot", "settings.ini")
	}
	if d := os.Getenv("USERPROFILE"); d != "" {
		return filepath.Join(d, "Dot", "settings.ini")
	}
	if d := os.Getenv("HOME"); d != "" {
		return filepath.Join(d, ".config", "dot", "settings.ini")
	}
	return ""
}

func (a *App) loadSettings() {
	p := settingsPath()
	if p == "" {
		return
	}
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return
	}
	defer applyUserKeys()
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		i := strings.IndexByte(ln, '=')
		if ln == "" || ln[0] == '#' || i < 0 {
			a.extraSettings += ln + "\n"
			continue
		}
		k, v := strings.TrimSpace(ln[:i]), strings.TrimSpace(ln[i+1:])
		n, _ := strconv.Atoi(v)
		if strings.HasPrefix(k, "key.") && v != "" { // key.ctrl+alt+x=command.id | none
			userKeys[strings.ToLower(k[4:])] = v
			continue
		}
		if k == "theme" {
			for i, t := range themes {
				if strings.EqualFold(t.Name, v) {
					themeIdx = i
					paletteRGB = t.C
				}
			}
			continue
		}
		if strings.HasPrefix(k, "alias.") { // alias.foo=py  -> .foo files get the .py icon
			if r, ok := iconExt[strings.ToLower(v)]; ok {
				iconExt[strings.ToLower(k[6:])] = r
			}
			continue
		}
		switch k {
		case "icons": // auto | on | off
			for i, m := range []string{"auto", "on", "off"} {
				if v == m {
					iconMode = i
				}
			}
		case "sidebar":
			a.sideVis = v == "1"
		case "panel":
			a.panelVis = v == "1"
		case "side_width":
			if n >= 14 && n <= 60 {
				a.sideW = n
			}
		case "panel_height":
			if n >= 4 && n <= 60 {
				a.panelH = n
			}
		case "shell":
			for _, s := range shellChoices() {
				if s == v {
					a.shell = v
				}
			}
		}
	}
}

func (a *App) saveSettings() {
	p := settingsPath()
	if p == "" {
		return
	}
	b2s := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	s := "# Dot settings (edited by Settings in Dot, or by hand)\n" +
		"icons=" + []string{"auto", "on", "off"}[iconMode] + "\n" +
		"sidebar=" + b2s(a.sideVis) + "\n" +
		"panel=" + b2s(a.panelVis) + "\n" +
		"side_width=" + itoa(a.sideW) + "\n" +
		"panel_height=" + itoa(a.panelH) + "\n" +
		"shell=" + a.shell + "\n" +
		"theme=" + themes[themeIdx].Name + "\n" + a.keySettings() + a.extraSettings
	os.MkdirAll(filepath.Dir(p), 0755)
	ioutil.WriteFile(p, []byte(s), 0644)
}

func (a *App) keySettings() string {
	var ks []string
	for k := range userKeys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	out := ""
	for _, k := range ks {
		out += "key." + k + "=" + userKeys[k] + "\n"
	}
	return out
}
