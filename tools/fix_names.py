#!/usr/bin/env python3
"""Add the full set of name records Windows needs to install a font. usage: fix_names.py font.ttf"""
import sys
from fontTools.ttLib import TTFont
f = TTFont(sys.argv[1])
n = f["name"]
rec = {0: "Dot icon font (icons from vscode-icons, CC BY-SA 4.0)", 1: "DotIcons", 2: "Regular",
       3: "Dot;DotIcons;1.0", 4: "DotIcons", 5: "Version 1.0", 6: "DotIcons"}
n.names = [x for x in n.names if x.nameID not in rec]
for pid, eid, lid in ((3, 1, 0x409), (1, 0, 0)):
    for k, v in rec.items():
        n.setName(v, k, pid, eid, lid)
f["OS/2"].fsType = 0
f["post"].formatType = 3.0 if f["post"].formatType not in (2.0, 3.0) else f["post"].formatType
f.save(sys.argv[1])
