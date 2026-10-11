package src

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Image viewer: draws a picture with half-block characters (each cell = 2 pixels:
// the "▀" foreground is the top pixel, the background is the bottom pixel).
//  - true-colour terminals (Windows Terminal, Windows 10): full 24-bit colour
//  - old consoles (Win 7/8/XP): the 16 console colours are switched to the standard VGA
//    palette while the viewer is open and the picture is dithered to it.

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true}

func isImagePath(p string) bool { return imageExts[strings.ToLower(filepath.Ext(p))] }

// standard VGA colours (console index order is BGR: 0 black,1 blue,2 green,3 cyan,...)
var vgaPalette = [16]uint32{
	0x000000, 0x000080, 0x008000, 0x008080, 0x800000, 0x800080, 0x808000, 0xc0c0c0,
	0x808080, 0x0000ff, 0x00ff00, 0x00ffff, 0xff0000, 0xff00ff, 0xffff00, 0xffffff,
}

// ImageView is a full-screen overlay.
type ImageView struct {
	path      string
	files     []string // images in the same folder, for next / previous
	idx       int
	img       *image.RGBA
	w, h      int // pixel size of img
	origW     int
	origH     int
	fileSize  int64
	err       string
	zoom      float64 // 1 = fit to window
	cx, cy    float64 // view centre in source pixels
	trueColor bool
	vw, vh    int // last drawn viewport (cells, pixel rows)
}

func (a *App) openImage(path string) {
	v := &ImageView{zoom: 1}
	if t, ok := a.be.(interface{ TrueColor() bool }); ok {
		v.trueColor = t.TrueColor()
	}
	if !v.trueColor {
		if p, ok := a.be.(interface{ ApplyPalette(*[16]uint32) }); ok {
			p.ApplyPalette(&vgaPalette)
		}
	}
	v.files = siblingImages(path)
	for i, f := range v.files {
		if f == path {
			v.idx = i
		}
	}
	v.load(path)
	a.overlay = v
}

func siblingImages(path string) []string {
	ents, err := readDir(filepath.Dir(path))
	if err != nil {
		return []string{path}
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && isImagePath(e.Name()) {
			out = append(out, filepath.Join(filepath.Dir(path), e.Name()))
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{path}
	}
	return out
}

func (v *ImageView) load(path string) {
	v.path, v.err, v.img, v.zoom = path, "", nil, 1
	f, err := os.Open(path)
	if err != nil {
		v.err = err.Error()
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil {
		v.fileSize = st.Size()
	}
	src, _, err := image.Decode(f)
	if err != nil {
		v.err = "Cannot decode this image (supported: PNG, JPEG, GIF): " + err.Error()
		return
	}
	b := src.Bounds()
	v.origW, v.origH = b.Dx(), b.Dy()
	// keep memory sane: shrink huge pictures to at most ~4M pixels
	step := 1
	for (b.Dx()/step)*(b.Dy()/step) > 4000000 {
		step++
	}
	if step == 1 {
		rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
		v.img = rgba
	} else {
		w, h := b.Dx()/step, b.Dy()/step
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				rgba.Set(x, y, src.At(b.Min.X+x*step, b.Min.Y+y*step))
			}
		}
		v.img = rgba
	}
	v.w, v.h = v.img.Bounds().Dx(), v.img.Bounds().Dy()
	v.cx, v.cy = float64(v.w)/2, float64(v.h)/2
}

func (v *ImageView) close(a *App) {
	a.overlay = nil
	if !v.trueColor {
		if p, ok := a.be.(interface{ ApplyPalette(*[16]uint32) }); ok {
			p.ApplyPalette(nil)
		}
	}
}

func (v *ImageView) step(a *App, d int) {
	if len(v.files) < 2 {
		return
	}
	v.idx = (v.idx + d + len(v.files)) % len(v.files)
	v.load(v.files[v.idx])
}

// scale = screen pixels per source pixel
func (v *ImageView) scale() float64 {
	if v.img == nil || v.vw < 1 || v.vh < 1 {
		return 1
	}
	fx, fy := float64(v.vw)/float64(v.w), float64(v.vh)/float64(v.h)
	f := fx
	if fy < f {
		f = fy
	}
	if f > 8 { // do not blow tiny icons up absurdly in "fit"
		f = 8
	}
	return f * v.zoom
}

func (v *ImageView) Key(a *App, ev Event) {
	k := ev.Key
	if k == "" && ev.Rune != 0 {
		k = string(ev.Rune)
	}
	pan := 0.1 / v.zoom
	switch k {
	case "esc", "q", "enter":
		v.close(a)
	case "+", "=", "ctrl+up":
		v.zoom *= 1.25
	case "-", "_", "ctrl+down":
		v.zoom /= 1.25
		if v.zoom < 1 {
			v.zoom = 1
		}
	case "f", "0":
		v.zoom, v.cx, v.cy = 1, float64(v.w)/2, float64(v.h)/2
	case "1": // actual size (1 image pixel = 1 screen pixel)
		if v.img != nil && v.vw > 0 {
			v.zoom = 1 / v.scale() * v.zoom
		}
	case "left", "h":
		v.cx -= float64(v.w) * pan
	case "right", "l":
		v.cx += float64(v.w) * pan
	case "up", "k":
		v.cy -= float64(v.h) * pan
	case "down", "j":
		v.cy += float64(v.h) * pan
	case "n", "pgdn", "space", " ":
		v.step(a, 1)
	case "p", "pgup", "backspace":
		v.step(a, -1)
	case "o": // open the same file as text
		p := v.path
		v.close(a)
		a.openAsText(p)
	}
	if v.zoom > 64 {
		v.zoom = 64
	}
	if v.img != nil {
		if v.cx < 0 {
			v.cx = 0
		}
		if v.cy < 0 {
			v.cy = 0
		}
		if v.cx > float64(v.w) {
			v.cx = float64(v.w)
		}
		if v.cy > float64(v.h) {
			v.cy = float64(v.h)
		}
	}
}

func (v *ImageView) Mouse(a *App, ev Event) {
	switch ev.Action {
	case "wheelup":
		v.zoom *= 1.25
	case "wheeldown":
		v.zoom /= 1.25
		if v.zoom < 1 {
			v.zoom = 1
		}
	}
}

func nearestVGA(r, g, b int) int {
	best, bd := 0, 1<<30
	for i, c := range vgaPalette {
		// console index order is BGR, vgaPalette is stored RGB
		cr, cg, cb := int(c>>16&255), int(c>>8&255), int(c&255)
		d := (r-cr)*(r-cr)*3 + (g-cg)*(g-cg)*4 + (b-cb)*(b-cb)*2
		if d < bd {
			best, bd = i, d
		}
	}
	return best
}

var bayer4 = [4][4]int{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

// sample returns the colour of screen pixel (px,py) of the viewport, as RGB 0-255
func (v *ImageView) sample(px, py, vw, vh int, sc float64) (int, int, int) {
	// source coordinates of this screen pixel
	sx := v.cx + (float64(px)+0.5-float64(vw)/2)/sc
	sy := v.cy + (float64(py)+0.5-float64(vh)/2)/sc
	if sx < 0 || sy < 0 || sx >= float64(v.w) || sy >= float64(v.h) {
		return -1, 0, 0
	}
	box := 1 / sc // source pixels covered by one screen pixel
	n := 1
	if box > 1.5 {
		n = 3
		if box > 6 {
			n = 4
		}
	}
	var sr, sg, sb, cnt int
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			x := int(sx - box/2 + (float64(i)+0.5)*box/float64(n))
			y := int(sy - box/2 + (float64(j)+0.5)*box/float64(n))
			if x < 0 {
				x = 0
			}
			if y < 0 {
				y = 0
			}
			if x >= v.w {
				x = v.w - 1
			}
			if y >= v.h {
				y = v.h - 1
			}
			o := y*v.img.Stride + x*4
			p := v.img.Pix[o : o+4]
			a := int(p[3])
			// image.RGBA is premultiplied: composite over a checkerboard
			bgc := 60
			if ((px/4)+(py/2))%2 == 0 {
				bgc = 90
			}
			sr += int(p[0]) + bgc*(255-a)/255
			sg += int(p[1]) + bgc*(255-a)/255
			sb += int(p[2]) + bgc*(255-a)/255
			cnt++
		}
	}
	return clamp255(sr / cnt), clamp255(sg / cnt), clamp255(sb / cnt)
}

func clamp255(x int) int {
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return x
}

func (v *ImageView) Draw(a *App) {
	s := a.scr
	// colours used by the chrome: black background / white text work in every palette
	bg, fg, dim := Color(0), Color(15), Color(7)
	if v.trueColor {
		bg, fg, dim = cBg, cFg, cDim
	}
	s.Fill(0, 0, a.W, a.H, ' ', fg, bg)
	// header + footer
	name := filepath.Base(v.path)
	info := ""
	if v.img != nil {
		info = fmt.Sprintf("%dx%d  %s  zoom %d%%", v.origW, v.origH, humanSize(v.fileSize), int(v.scale()*100))
		if len(v.files) > 1 {
			info += fmt.Sprintf("  [%d/%d]", v.idx+1, len(v.files))
		}
	}
	s.Fill(0, 0, a.W, 1, ' ', fg, bg)
	s.PutN(1, 0, a.W-2, name+"   "+info, fg, bg)
	help := "+/- zoom  arrows pan  f fit  1 actual  n/p next/prev  o open as text  Esc close"
	s.PutN(1, a.H-1, a.W-2, help, dim, bg)
	vx, vy := 0, 1
	vw, vhCells := a.W, a.H-2
	if v.err != "" {
		s.PutN(2, vy+1, a.W-4, v.err, 12, bg)
		if v.trueColor {
			s.PutN(2, vy+1, a.W-4, v.err, cRed, bg)
		}
		return
	}
	if v.img == nil || vw < 1 || vhCells < 1 {
		return
	}
	vh := vhCells * 2
	v.vw, v.vh = vw, vh
	sc := v.scale()
	for cy := 0; cy < vhCells; cy++ {
		for cx := 0; cx < vw; cx++ {
			r1, g1, b1 := v.sample(cx, cy*2, vw, vh, sc)
			r2, g2, b2 := v.sample(cx, cy*2+1, vw, vh, sc)
			if r1 < 0 && r2 < 0 {
				continue
			}
			if r1 < 0 {
				r1, g1, b1 = 0, 0, 0
			}
			if r2 < 0 {
				r2, g2, b2 = 0, 0, 0
			}
			if v.trueColor {
				s.SetRGB(vx+cx, vy+cy, '▀', uint32(r1<<16|g1<<8|b1), uint32(r2<<16|g2<<8|b2))
				continue
			}
			// old console: ordered dithering into the 16 VGA colours
			t := (bayer4[cy&3][cx&3] - 8) * 4
			f := nearestVGA(clamp255(r1+t), clamp255(g1+t), clamp255(b1+t))
			g := nearestVGA(clamp255(r2+t), clamp255(g2+t), clamp255(b2+t))
			s.Set(vx+cx, vy+cy, '▀', Color(vgaToConsole(f)), Color(vgaToConsole(g)))
		}
	}
}

// vgaPalette is stored in RGB order and written to the console as given, so the console
// index equals the palette index.
func vgaToConsole(i int) int { return i }

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
