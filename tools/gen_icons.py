#!/usr/bin/env python3
"""Build DotIcons.ttf (colour COLR/CPAL icon font) + icons_data.go from the vscode-icons SVG set.
usage: gen_icons.py <vscode-icons checkout> <out dir>
after building run: python tools/fix_names.py DotIcons.ttf
"""
import os, re, sys, math
import xml.etree.ElementTree as ET
from fontTools.fontBuilder import FontBuilder
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.cu2quPen import Cu2QuPen
from fontTools.svgLib.path import parse_path
from fontTools.colorLib.builder import buildCOLR, buildCPAL

SRC, OUT = sys.argv[1], sys.argv[2]
ICONS = os.path.join(SRC, "icons")
ADV, W, H, YOFF = 700, 750, 850, 40  # advance, icon box width/height, baseline offset

NAMED = {"white": "ffffff", "black": "000000", "red": "ff0000", "green": "008000", "blue": "0000ff",
         "yellow": "ffff00", "gray": "808080", "grey": "808080", "orange": "ffa500"}

GRAD = {}

def parse_color(v, inherited):
    if v is None: return inherited
    v = v.strip().lower()
    mg = re.match(r"url\(#([^)]+)\)", v)
    if mg and mg.group(1) in GRAD: return GRAD[mg.group(1)]
    if v in ("none", "transparent"): return None
    if v in ("currentcolor", "inherit"): return inherited or "d4d4d4"
    if v.startswith("url("): return inherited or "808080"
    if v in NAMED: return NAMED[v]
    m = re.match(r"#([0-9a-f]{6}|[0-9a-f]{3})", v)
    if m:
        h = m.group(1)
        if len(h) == 3: h = "".join(c * 2 for c in h)
        return h
    m = re.match(r"rgb\((\d+),\s*(\d+),\s*(\d+)", v)
    if m: return "%02x%02x%02x" % tuple(int(x) for x in m.groups())
    return inherited

def mat_mul(a, b):  # a*b, matrices (a,b,c,d,e,f)
    return (a[0]*b[0]+a[2]*b[1], a[1]*b[0]+a[3]*b[1], a[0]*b[2]+a[2]*b[3], a[1]*b[2]+a[3]*b[3],
            a[0]*b[4]+a[2]*b[5]+a[4], a[1]*b[4]+a[3]*b[5]+a[5])

def parse_transform(t):
    m = (1, 0, 0, 1, 0, 0)
    if not t: return m
    for name, args in re.findall(r"(\w+)\s*\(([^)]*)\)", t):
        n = [float(x) for x in re.split(r"[\s,]+", args.strip()) if x]
        if name == "translate": t2 = (1, 0, 0, 1, n[0], n[1] if len(n) > 1 else 0)
        elif name == "scale": t2 = (n[0], 0, 0, n[1] if len(n) > 1 else n[0], 0, 0)
        elif name == "rotate":
            a = math.radians(n[0]); c, s = math.cos(a), math.sin(a)
            t2 = (c, s, -s, c, 0, 0)
            if len(n) == 3:
                t2 = mat_mul(mat_mul((1, 0, 0, 1, n[1], n[2]), t2), (1, 0, 0, 1, -n[1], -n[2]))
        elif name == "matrix" and len(n) == 6: t2 = tuple(n)
        else: continue
        m = mat_mul(m, t2)
    return m

def fnum(e, k, d=0.0):
    try: return float(re.sub(r"[a-z%]+$", "", e.get(k, str(d))))
    except ValueError: return d

def shape_path(e, tag):
    if tag == "path": return e.get("d")
    if tag == "rect":
        x, y, w, h = fnum(e, "x"), fnum(e, "y"), fnum(e, "width"), fnum(e, "height")
        rx = fnum(e, "rx", fnum(e, "ry", 0)); ry = fnum(e, "ry", rx)
        if rx <= 0 and ry <= 0:
            return "M%g %gh%gv%gh%gz" % (x, y, w, h, -w)
        rx, ry = min(rx, w/2), min(ry, h/2)
        return ("M%g %gh%ga%g %g 0 0 1 %g %gv%ga%g %g 0 0 1 %g %gh%ga%g %g 0 0 1 %g %gv%ga%g %g 0 0 1 %g %gz" %
                (x+rx, y, w-2*rx, rx, ry, rx, ry, h-2*ry, rx, ry, -rx, ry, -(w-2*rx), rx, ry, -rx, -ry, -(h-2*ry), rx, ry, rx, -ry))
    if tag in ("circle", "ellipse"):
        cx, cy = fnum(e, "cx"), fnum(e, "cy")
        rx = fnum(e, "r") if tag == "circle" else fnum(e, "rx")
        ry = rx if tag == "circle" else fnum(e, "ry")
        return "M%g %ga%g %g 0 1 0 %g 0a%g %g 0 1 0 %g 0z" % (cx-rx, cy, rx, ry, 2*rx, rx, ry, -2*rx)
    if tag in ("polygon", "polyline"):
        pts = [float(x) for x in re.split(r"[\s,]+", e.get("points", "").strip()) if x]
        if len(pts) < 4: return None
        d = "M%g %g" % (pts[0], pts[1]) + "".join("L%g %g" % (pts[i], pts[i+1]) for i in range(2, len(pts)-1, 2))
        return d + "z"
    return None

def strip(tag): return tag.split("}")[-1]

def svg_shapes(path):
    """-> list of (colorhex, d, matrix)"""
    root = ET.parse(path).getroot()
    vb = [float(x) for x in re.split(r"[\s,]+", root.get("viewBox", "0 0 32 32").strip())]
    out = []
    GRAD.clear()
    for g in root.iter():
        if strip(g.tag) in ("linearGradient", "radialGradient") and g.get("id"):
            stops = [c for c in g if strip(c.tag) == "stop"]
            if stops:
                st0 = dict(kv.split(":", 1) for kv in stops[0].get("style", "").split(";") if ":" in kv)
                c = parse_color(st0.get("stop-color", stops[0].get("stop-color")), "808080")
                GRAD[g.get("id")] = c
    def walk(e, fill, mat):
        tag = strip(e.tag)
        if tag in ("defs", "clipPath", "mask", "title", "desc", "style", "metadata"): return
        st = dict(kv.split(":", 1) for kv in e.get("style", "").split(";") if ":" in kv)
        f = parse_color(st.get("fill", e.get("fill")), fill)
        if "fill" not in st and e.get("fill") is None and tag in ("path", "rect", "circle", "ellipse", "polygon") and f is None:
            f = fill
        m = mat_mul(mat, parse_transform(e.get("transform")))
        d = shape_path(e, tag)
        if d and f and st.get("display") != "none" and e.get("display") != "none":
            out.append((f, d, m))
        for c in e: walk(c, f if (e.get("fill") or "fill" in st) else fill, m)
    walk(root, "000000", (1, 0, 0, 1, 0, 0))
    return out, vb

def draw(shape, vb, pen_factory):
    color, d, m = shape
    pen = pen_factory()
    sx, sy = W / vb[2], H / vb[3]
    # svg (x,y) -> apply m -> font: x*sx, (vb_h - y)*sy + YOFF
    base = (sx, 0, 0, -sy, -vb[0]*sx, (vb[1]+vb[3])*sy + YOFF)
    full = mat_mul(base, m)
    tp = TransformPen(Cu2QuPen(pen, 1.0, reverse_direction=False), full)
    parse_path(d, tp)
    return pen

def build_glyph(shapes, vb):
    """returns (base glyph, [(layer glyph, color)])"""
    layers = []
    for sh in shapes:
        try:
            p = draw(sh, vb, lambda: TTGlyphPen(None))
            g = p.glyph()
        except Exception:
            continue
        if g.numberOfContours == 0: continue
        if layers and layers[-1][1] == sh[0]:
            # merge consecutive same colour: redraw into combined pen
            layers[-1][2].append(sh)
        else:
            layers.append([None, sh[0], [sh]])
    res = []
    for _, color, shs in layers:
        pen = TTGlyphPen(None)
        for sh in shs:
            try:
                draw_into = draw(sh, vb, lambda: pen)
            except Exception:
                pass
        g = pen.glyph()
        if g.numberOfContours: res.append((g, color))
    allpen = TTGlyphPen(None)
    for sh in shapes:
        try: draw(sh, vb, lambda: allpen)
        except Exception: pass
    return allpen.glyph(), res

# ---- manifest ---------------------------------------------------------------
def read(p): return open(os.path.join(SRC, "src", "iconsManifest", p), encoding="utf8").read()

lang_ext = {}
lt = read("languages.ts")
starts = [(m.group(1), m.start()) for m in re.finditer(r"\n  (\w+):\s*\{", lt)]
for n, (name, st) in enumerate(starts):
    body = lt[st:(starts[n+1][1] if n+1 < len(starts) else len(lt))]
    k = re.search(r"knownExtensions:\s*\[([^\]]*)\]", body, re.S)
    if k: lang_ext[name] = re.findall(r"'([^']+)'", k.group(1))

def chunks(text):
    pos = [m.start() for m in re.finditer(r"icon: '", text)]
    for i, p in enumerate(pos):
        yield text[p:(pos[i+1] if i+1 < len(pos) else len(text))]

ext_map, file_map, folder_map = {}, {}, {}
for ch in chunks(read("supportedExtensions.ts")):
    name = re.match(r"icon: '([^']+)'", ch).group(1)
    if "disabled: true" in ch: continue
    if not os.path.exists(os.path.join(ICONS, "file_type_%s.svg" % name)): continue
    exts = []
    m = re.search(r"extensions:\s*\[([^\]]*)\]", ch, re.S)
    if m: exts = re.findall(r"'([^']+)'", m.group(1))
    isfile = "filename: true" in ch
    for l in re.findall(r"languages\.(\w+)", ch):
        if not isfile: exts += lang_ext.get(l, [])
    for e in exts:
        if isfile: file_map.setdefault(e.lower(), name)
        else: ext_map.setdefault(e.lower().lstrip("."), name)

for ch in chunks(read("supportedFolders.ts")):
    name = re.match(r"icon: '([^']+)'", ch).group(1)
    if "disabled: true" in ch: continue
    if not os.path.exists(os.path.join(ICONS, "folder_type_%s.svg" % name)): continue
    m = re.search(r"extensions:\s*\[([^\]]*)\]", ch, re.S)
    for e in (re.findall(r"'([^']+)'", m.group(1)) if m else []):
        folder_map.setdefault(e.lower(), name)

# ---- which svgs we need ------------------------------------------------------
need_files = sorted(set(ext_map.values()) | set(file_map.values()))
need_folders = sorted(set(folder_map.values()))
glyph_src = {}   # glyph key -> svg path
for n in need_files: glyph_src["f:" + n] = os.path.join(ICONS, "file_type_%s.svg" % n)
for n in need_folders:
    glyph_src["d:" + n] = os.path.join(ICONS, "folder_type_%s.svg" % n)
    op = os.path.join(ICONS, "folder_type_%s_opened.svg" % n)
    glyph_src["o:" + n] = op if os.path.exists(op) else glyph_src["d:" + n]
glyph_src["f:"] = os.path.join(ICONS, "default_file.svg")
glyph_src["d:"] = os.path.join(ICONS, "default_folder.svg")
glyph_src["o:"] = os.path.join(ICONS, "default_folder_opened.svg")
glyph_src["d:#root"] = os.path.join(ICONS, "default_root_folder.svg")
glyph_src["o:#root"] = os.path.join(ICONS, "default_root_folder_opened.svg")

keys = sorted(glyph_src)
cp = {k: 0xE000 + i for i, k in enumerate(keys)}
assert len(keys) < 6400, len(keys)

glyph_order = [".notdef"]
glyphs = {".notdef": TTGlyphPen(None).glyph()}
cmap, colr, palette, pal_idx = {}, {}, [], {}
dominant = {}
for k in keys:
    try:
        shapes, vb = svg_shapes(glyph_src[k])
        base, layers = build_glyph(shapes, vb)
    except Exception as ex:
        print("skip", k, ex); continue
    if not layers: continue
    gn = "u%04X" % cp[k]
    glyph_order.append(gn); glyphs[gn] = base; cmap[cp[k]] = gn
    lay = []
    best, bestarea = None, -1
    for i, (g, color) in enumerate(layers):
        ln = "%s.%d" % (gn, i)
        glyph_order.append(ln); glyphs[ln] = g
        if color not in pal_idx:
            pal_idx[color] = len(palette)
            palette.append((int(color[0:2], 16)/255, int(color[2:4], 16)/255, int(color[4:6], 16)/255, 1.0))
        lay.append((ln, pal_idx[color]))
        g.recalcBounds(None)
        area = (g.xMax - g.xMin) * (g.yMax - g.yMin) if g.numberOfContours else 0
        if color not in ("ffffff", "000000") and area > bestarea: best, bestarea = color, area
    colr[gn] = lay
    dominant[cp[k]] = best or layers[0][1]

fb = FontBuilder(1000, isTTF=True)
fb.setupGlyphOrder(glyph_order)
fb.setupCharacterMap(cmap)
fb.setupGlyf(glyphs)
fb.setupHorizontalMetrics({g: (ADV if g == ".notdef" or "." not in g else ADV, 0) for g in glyph_order})
fb.setupHorizontalHeader(ascent=900, descent=-250)
fb.setupNameTable({"familyName": "DotIcons", "styleName": "Regular"})
fb.setupOS2(sTypoAscender=900, sTypoDescender=-250, usWinAscent=900, usWinDescent=250, achVendID="DOTI")
fb.setupPost(isFixedPitch=1)
fb.font["COLR"] = buildCOLR(colr, version=0)
fb.font["CPAL"] = buildCPAL([palette])
os.makedirs(OUT, exist_ok=True)
fb.save(os.path.join(OUT, "DotIcons.ttf"))

# nearest palette slot (foreground-ish slots only) for the no-font fallback
slots = {2: 0x6a9955, 3: 0x4ec9b0, 4: 0xf44747, 5: 0xc586c0, 6: 0xdcdcaa, 7: 0xd4d4d4, 9: 0x569cd6, 10: 0xb5cea8, 11: 0x9cdcfe, 12: 0xce9178, 15: 0x858585}
def nearest(hexs):
    r, g, b = int(hexs[0:2], 16), int(hexs[2:4], 16), int(hexs[4:6], 16)
    return min(slots, key=lambda s: (r-(slots[s]>>16))**2 + (g-((slots[s]>>8)&255))**2 + (b-(slots[s]&255))**2)

def has(k): return k in cmap.values() or ("u%04X" % cp[k]) in cmap.values()
ok = lambda k: k in cp and cmap.get(cp[k])
with open(os.path.join(OUT, "icons_data.go"), "w") as f:
    f.write("// Code generated by tools/gen_icons.py from the vscode-icons set (CC BY-SA 4.0). DO NOT EDIT.\npackage main\n\n")
    f.write("var iconExt = map[string]rune{\n")
    for e, n in sorted(ext_map.items()):
        if ok("f:" + n): f.write('\t"%s": 0x%X,\n' % (e.replace("\\", "\\\\"), cp["f:" + n]))
    f.write("}\n\nvar iconFile = map[string]rune{\n")
    for e, n in sorted(file_map.items()):
        if ok("f:" + n): f.write('\t"%s": 0x%X,\n' % (e.replace("\\", "\\\\"), cp["f:" + n]))
    f.write("}\n\nvar iconFolder = map[string][2]rune{\n")
    for e, n in sorted(folder_map.items()):
        if ok("d:" + n): f.write('\t"%s": {0x%X, 0x%X},\n' % (e, cp["d:" + n], cp["o:" + n]))
    f.write("}\n\nconst (\n\ticonDefFile = 0x%X\n\ticonDefDir = 0x%X\n\ticonDefDirOpen = 0x%X\n\ticonRoot = 0x%X\n\ticonRootOpen = 0x%X\n)\n\n" %
            (cp["f:"], cp["d:"], cp["o:"], cp["d:#root"], cp["o:#root"]))
    f.write("var iconSlot = map[rune]uint8{\n")
    for c, col in sorted(dominant.items()): f.write("\t0x%X: %d,\n" % (c, nearest(col)))
    f.write("}\n")
print("glyphs:", len(keys), "ext:", len(ext_map), "files:", len(file_map), "folders:", len(folder_map), "palette:", len(palette))
