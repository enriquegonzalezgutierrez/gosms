#!/usr/bin/env python3
"""Audita transparencias entre un pack HD y la extracción original.

Uso:
    python tools/audit_alpha.py assets/<ROM>/hd assets/<ROM>/original

No modifica nada: compara la cobertura alfa de cada tile y lista los que
pierden o ganan transparencia, con los objetos que los usan.
"""

import json
import os
import sys
from collections import defaultdict

import numpy as np
from PIL import Image


def load(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def alpha_fraction(tile):
    alpha = np.asarray(tile.convert("RGBA"))[..., 3]
    return float((alpha > 0).mean()) if alpha.size else 0.0


def main():
    if len(sys.argv) < 3:
        print("Uso: audit_alpha.py <hd_dir> <original_dir>")
        return 2
    hd_dir, original_dir = sys.argv[1], sys.argv[2]
    original = load(os.path.join(original_dir, "manifest.json"))
    hd = load(os.path.join(hd_dir, "manifest.json"))
    conflicts_path = os.path.join(hd_dir, "conflicts.json")
    conflicts = load(conflicts_path) if os.path.exists(conflicts_path) else {}
    scale = int(conflicts.get("scale", 4) or 4)
    tile = 8 * scale
    cell = 12 * scale
    pad = 2 * scale

    original_sheet = Image.open(os.path.join(original_dir, "spritesheet.png")).convert("RGBA")
    hd_sheet = Image.open(os.path.join(hd_dir, "spritesheet.png")).convert("RGBA")
    original_cols = original_sheet.width // 12
    hd_cols = hd_sheet.width // cell

    hd_by_index = {item["index"] for item in hd["tiles"]}
    by_index = defaultdict(list)
    is_sprite = {}
    for item in original["tiles"]:
        is_sprite[item["index"]] = bool(item.get("is_sprite"))
    for obj in original.get("objects", []):
        for placement in obj.get("tiles", []):
            by_index[placement["index"]].append(obj)

    rows = []
    missing = 0
    for item in original["tiles"]:
        idx = item["index"]
        if idx not in hd_by_index:
            missing += 1
            continue
        ox = (idx % original_cols) * 12 + 2
        oy = (idx // original_cols) * 12 + 2
        hx = (idx % hd_cols) * cell + pad
        hy = (idx // hd_cols) * cell + pad
        orig = original_sheet.crop((ox, oy, ox + 8, oy + 8))
        rem = hd_sheet.crop((hx, hy, hx + tile, hy + tile))
        oa = alpha_fraction(orig)
        ha = alpha_fraction(rem)
        diff = ha - oa
        if oa == 0 and ha > 0:
            rows.append(("LOST", diff, idx, oa, ha))
        elif oa > 0 and ha == 0:
            rows.append(("GAINED", diff, idx, oa, ha))
        elif abs(diff) >= 0.02:
            rows.append(("PARTIAL", diff, idx, oa, ha))

    lost = [r for r in rows if r[0] == "LOST"]
    gained = [r for r in rows if r[0] == "GAINED"]
    partial = [r for r in rows if r[0] == "PARTIAL"]
    print(f"tiles originales: {len(original['tiles'])}")
    print(f"tiles en el pack: {len(hd_by_index)}")
    print(f"ausentes en el pack: {missing}")
    print(f"transparencia perdida (original transparente, HD opaco): {len(lost)}")
    print(f"transparencia añadida (original opaco, HD transparente): {len(gained)}")
    print(f"diferencias parciales de alfa >= 0.02: {len(partial)}")
    for tag, items in (("LOST", lost), ("GAINED", gained), ("PARTIAL", partial)):
        if not items:
            continue
        print(f"\n-- {tag} --")
        for _, diff, idx, oa, ha in sorted(items, key=lambda r: -abs(r[1]))[:40]:
            objs = by_index.get(idx, [])
            labels = ", ".join(f"{o.get('id')}({o.get('kind')})" for o in objs[:3])
            kind = "spr" if is_sprite.get(idx) else "bg"
            print(f"{kind}_{idx:04d} alfa {oa:.2f} -> {ha:.2f} (delta {diff:+.2f}) objs: {labels or '-'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
