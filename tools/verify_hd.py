#!/usr/bin/env python3
"""Comprueba que un pack HD no haya horneado píxeles de otro objeto.

Uso:
    python tools/verify_hd.py "assets/<ROM>/hd" "assets/<ROM>/original"

El chequeo compara cada tile original con su versión HD y marca:
  * tiles que ganan una cantidad anómala de rojo/blanco (contaminación típica
    de un sprite horneado dentro de un tile de fondo),
  * tiles cuyo color medio cambia radicalmente,
  * tiles que desaparecen (transparentes) en el pack,
  * posiciones duplicadas dentro de un mismo objeto (sprites solapados).
"""

import json
import os
import sys
from collections import Counter, defaultdict

from PIL import Image


def load_json(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def sheet_cell(sheet, index, cell, padding, tile, cols):
    x = (index % cols) * cell + padding
    y = (index // cols) * cell + padding
    return sheet.crop((x, y, x + tile, y + tile)).convert("RGBA")


def stats(tile):
    px = list(tile.getdata())
    opaque = [(r, g, b) for r, g, b, a in px if a > 0]
    if not opaque:
        return {
            "opaque": 0,
            "red": 0,
            "white": 0,
            "mean": (0, 0, 0),
        }
    red = sum(1 for r, g, b in opaque if r > 170 and g < 90 and b < 90)
    white = sum(1 for r, g, b in opaque if r > 200 and g > 200 and b > 200)
    n = len(opaque)
    return {
        "opaque": n,
        "red": red,
        "white": white,
        "mean": tuple(sum(c[i] for c in opaque) / n for i in range(3)),
    }


def color_distance(a, b):
    return sum((a[i] - b[i]) ** 2 for i in range(3)) ** 0.5


def main():
    if len(sys.argv) < 3:
        print("Uso: verify_hd.py <hd_dir> <original_dir>")
        return 2

    hd_dir, original_dir = sys.argv[1], sys.argv[2]
    original_manifest = load_json(os.path.join(original_dir, "manifest.json"))
    hd_manifest = load_json(os.path.join(hd_dir, "manifest.json"))
    conflicts_path = os.path.join(hd_dir, "conflicts.json")
    conflicts = load_json(conflicts_path) if os.path.exists(conflicts_path) else {"tiles": []}
    scale = int(conflicts.get("scale", 4) or 4)
    tile = 8 * scale
    cell = 12 * scale
    padding = 2 * scale

    original_sheet = Image.open(os.path.join(original_dir, "spritesheet.png")).convert("RGBA")
    hd_sheet = Image.open(os.path.join(hd_dir, "spritesheet.png")).convert("RGBA")
    original_cols = original_sheet.width // 12
    hd_cols = hd_sheet.width // cell

    hd_by_index = {item["index"]: item for item in hd_manifest["tiles"]}
    conflict_by_index = {item["index"]: item for item in conflicts.get("tiles", [])}

    suspicious = []
    missing = []
    mean_shift = []
    for item in original_manifest["tiles"]:
        index = item["index"]
        if index not in hd_by_index:
            missing.append(index)
            continue
        original = stats(sheet_cell(original_sheet, index, 12, 2, 8, original_cols))
        hd = stats(sheet_cell(hd_sheet, index, cell, padding, tile, hd_cols))
        if original["opaque"] == 0:
            continue
        # Compara proporciones, no conteos: el pack 4x tiene 16 veces más
        # píxeles. Un salto grande y simultáneo de rojo y blanco es el patrón
        # típico de un sprite (Mickey, enemigo) horneado en el fondo.
        orig_op = max(1, original["opaque"])
        hd_op = max(1, hd["opaque"])
        dr = hd["red"] / hd_op - original["red"] / orig_op
        dw = hd["white"] / hd_op - original["white"] / orig_op
        if dr > 0.08 and dw > 0.08:
            suspicious.append((index, original, hd, "red/white"))
        elif hd["opaque"] == 0:
            suspicious.append((index, original, hd, "transparente"))
        elif color_distance(original["mean"], hd["mean"]) > 110:
            mean_shift.append((index, original, hd))

    dup_objects = []
    for obj in original_manifest.get("objects", []):
        counts = Counter((p["x"], p["y"]) for p in (obj.get("tiles") or []))
        dup = {pos: n for pos, n in counts.items() if n > 1}
        if dup:
            dup_objects.append((obj["id"], obj.get("kind"), len((obj.get("tiles") or [])), len(dup)))

    print(f"tiles originales: {len(original_manifest['tiles'])}")
    print(f"tiles en el pack: {len(hd_by_index)}")
    print(f"tiles ausentes: {len(missing)}")
    print(f"tiles sospechosos por rojo/blanco o transparencia: {len(suspicious)}")
    print(f"tiles con cambio de color medio > 110: {len(mean_shift)}")
    print(f"objetos con posiciones duplicadas: {len(dup_objects)}")

    if suspicious:
        print("\n-- sospechosos --")
        for index, original, hd, reason in suspicious[:30]:
            conflict = conflict_by_index.get(index)
            owner = conflict.get("kept") if conflict else "-"
            print(
                f"{index:4d} {reason:14s} original(r={original['red']},w={original['white']},n={original['opaque']}) "
                f"hd(r={hd['red']},w={hd['white']},n={hd['opaque']}) kept={owner}"
            )
    if mean_shift:
        print("\n-- cambios de color grandes --")
        for index, original, hd in mean_shift[:20]:
            print(
                f"{index:4d} original={tuple(round(c) for c in original['mean'])} "
                f"hd={tuple(round(c) for c in hd['mean'])}"
            )
    if dup_objects:
        print("\n-- objetos con posiciones duplicadas --")
        for oid, kind, tiles, dup in dup_objects[:20]:
            print(f"{oid} {kind} tiles={tiles} posiciones_duplicadas={dup}")

    return 1 if suspicious or dup_objects else 0


if __name__ == "__main__":
    raise SystemExit(main())
