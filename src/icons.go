package src

import (
	"os"
	"path/filepath"
	"strings"
)

// File icons come from the vscode-icons set, packed into the DotIcons.ttf colour font
// (private-use code points, see tools/gen_icons.py). Without the font installed we show a
// coloured square instead.

var iconMode = 0 // 0 auto (font detected), 1 on, 2 off
var iconFont bool

func detectIconFont() {
	if os.Getenv("DOT_ICONS") == "1" {
		iconFont = true
		return
	}
	for _, d := range []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"),
		filepath.Join(os.Getenv("WINDIR"), "Fonts"),
		"/usr/share/fonts", filepath.Join(os.Getenv("HOME"), ".local/share/fonts"),
	} {
		if d == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "DotIcons.ttf")); err == nil {
			iconFont = true
			return
		}
	}
}

func iconsOn() bool { return uni && (iconMode == 1 || (iconMode == 0 && iconFont)) }

func fileIconRune(name string) rune {
	low := strings.ToLower(filepath.Base(name))
	if r, ok := iconFile[low]; ok {
		return r
	}
	for i := 0; i < len(low); i++ {
		if low[i] == '.' && i+1 < len(low) {
			if r, ok := iconExt[low[i+1:]]; ok {
				return r
			}
		}
	}
	return iconDefFile
}

func fileIcon(name string) iconGlyph {
	r := fileIconRune(name)
	if iconsOn() {
		return iconGlyph{r, cFg}
	}
	if r == iconDefFile {
		return iconGlyph{'■', langForFile(name).Icon}
	}
	return iconGlyph{'■', Color(iconSlot[r])}
}

func folderIcon(name string, open, root bool) iconGlyph {
	if !iconsOn() {
		return iconGlyph{}
	}
	if root {
		if open {
			return iconGlyph{iconRootOpen, cFg}
		}
		return iconGlyph{iconRoot, cFg}
	}
	if p, ok := iconFolder[strings.ToLower(name)]; ok {
		if open {
			return iconGlyph{p[1], cFg}
		}
		return iconGlyph{p[0], cFg}
	}
	if open {
		return iconGlyph{iconDefDirOpen, cFg}
	}
	return iconGlyph{iconDefDir, cFg}
}

// iconNote explains in Settings why icons may not be showing.
func iconNote() string {
	switch {
	case !uni:
		return "  (this console cannot show icons)"
	case iconMode == 0 && !iconFont:
		return "  (font not found: set mode to on, or install DotIcons.ttf)"
	}
	return ""
}
