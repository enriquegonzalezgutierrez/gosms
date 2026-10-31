#!/usr/bin/env python3
"""Genera y recorta una hoja de las 33 poses únicas de Alex para Gemini.

La hoja mantiene una cuadrícula fija y un baseline común. Gemini remasteriza
la hoja completa (no pose a pose), lo que conserva mucho mejor el diseño del
personaje. Este script también recorta el resultado en los 33 objetos.
"""

import argparse
import json
import os
import sys

from PIL import Image, ImageDraw

GRID_COLUMNS = 6
GRID_ROWS = 6
CELL = 40                 # tamaño de celda en píxeles nativos
INNER = 32                # caja útil dentro de la celda
MARGIN = (CELL - INNER) // 2
SCALE = 8                 # resolución de la hoja para Gemini
GRID_COLOR = (255, 0, 255, 255)
BACKGROUND = (110, 110, 110, 255)


def paths(rom_name):
    root = os.path.join("assets", rom_name)
    original = os.path.join(root, "original")
    gemini = os.path.join(root, "ai-remaster", "gemini")
    return root, original, gemini


def load_poses(original_dir):
    with open(os.path.join(original_dir, "manifest.json"), "r", encoding="utf-8") as source:
        document = json.load(source)
    by_label = {}
    for obj in document.get("objects", []):
        if obj.get("kind") != "sprite_pose":
            continue
        label = obj.get("label", "")
        if label and label not in by_label:
            by_label[label] = obj
    return [by_label[label] for label in sorted(by_label)]


def build(rom_name):
    root, original_dir, gemini_dir = paths(rom_name)
    os.makedirs(gemini_dir, exist_ok=True)
    poses = load_poses(original_dir)
    if not poses:
        print("No hay poses sprite_pose en este manifest; se omite la hoja de Gemini.")
        return
    if len(poses) > GRID_COLUMNS * GRID_ROWS:
        sys.exit(f"Hay {len(poses)} poses y la cuadrícula solo tiene {GRID_COLUMNS * GRID_ROWS} celdas")

    sheet_w = GRID_COLUMNS * CELL * SCALE
    sheet_h = GRID_ROWS * CELL * SCALE
    sheet = Image.new("RGBA", (sheet_w, sheet_h), BACKGROUND)
    draw = ImageDraw.Draw(sheet)

    # Guías de celda: quedan en el margen y nunca entran en el recorte del objeto.
    for column in range(GRID_COLUMNS + 1):
        x = column * CELL * SCALE
        draw.line((x, 0, x, sheet_h), fill=GRID_COLOR, width=2)
    for row in range(GRID_ROWS + 1):
        y = row * CELL * SCALE
        draw.line((0, y, sheet_w, y), fill=GRID_COLOR, width=2)

    entries = []
    for index, obj in enumerate(poses):
        column = index % GRID_COLUMNS
        row = index // GRID_COLUMNS
        width = int(obj.get("width", 0))
        height = int(obj.get("height", 0))
        if width <= 0 or height <= 0 or width > INNER or height > INNER:
            sys.exit(f"Pose {obj['id']} no cabe en la celda: {width}x{height}")
        offset_x = MARGIN + (INNER - width) // 2
        offset_y = MARGIN + (INNER - height)
        source_path = os.path.join(original_dir, "objects", f"{obj['id']}.png")
        with Image.open(source_path) as source:
            pose = source.convert("RGBA").resize((width * SCALE, height * SCALE), Image.Resampling.NEAREST)
        sheet.alpha_composite(
            pose,
            ((column * CELL + offset_x) * SCALE, (row * CELL + offset_y) * SCALE),
        )
        entries.append({
            "index": index,
            "row": row,
            "column": column,
            "id": obj["id"],
            "label": obj.get("label", ""),
            "width": width,
            "height": height,
            "offset_x": offset_x,
            "offset_y": offset_y,
            "archive": f"assets/{rom_name}/ai-remaster/poses/{obj['id']}.png",
            "pack": f"assets/{rom_name}/remaster_workspace/objects/{obj['id']}.png",
        })

    sheet_path = os.path.join(gemini_dir, "poses_sheet.png")
    sheet.save(sheet_path)
    metadata = {
        "rom": rom_name,
        "sheet": f"assets/{rom_name}/ai-remaster/gemini/poses_sheet.png",
        "columns": GRID_COLUMNS,
        "rows": GRID_ROWS,
        "cell": CELL,
        "inner": INNER,
        "margin": MARGIN,
        "scale": SCALE,
        "background": list(BACKGROUND),
        "entries": entries,
    }
    with open(os.path.join(gemini_dir, "poses_sheet.json"), "w", encoding="utf-8") as output:
        json.dump(metadata, output, indent=2, ensure_ascii=False)
    print(f"Hoja: {sheet_path} ({sheet_w}x{sheet_h}), {len(entries)} poses")
    print(f"Metadatos: {os.path.join(gemini_dir, 'poses_sheet.json')}")


def slice_sheet(rom_name, generated_path, copy_to_pack=False):
    _, _, gemini_dir = paths(rom_name)
    with open(os.path.join(gemini_dir, "poses_sheet.json"), "r", encoding="utf-8") as source:
        metadata = json.load(source)
    with Image.open(generated_path) as source:
        sheet = source.convert("RGBA")
    expected_w = metadata["columns"] * metadata["cell"] * metadata["scale"]
    expected_h = metadata["rows"] * metadata["cell"] * metadata["scale"]
    factor_x = sheet.width / expected_w
    factor_y = sheet.height / expected_h
    if abs(factor_x - factor_y) > 0.01:
        sys.exit(f"La hoja generada tiene otra proporción: {sheet.size} frente a {expected_w}x{expected_h}")
    factor = factor_x
    print(f"Recortando {sheet.width}x{sheet.height} (factor {factor:.3f})")

    for entry in metadata["entries"]:
        x = (entry["column"] * metadata["cell"] + entry["offset_x"]) * metadata["scale"] * factor
        y = (entry["row"] * metadata["cell"] + entry["offset_y"]) * metadata["scale"] * factor
        width = entry["width"] * metadata["scale"] * factor
        height = entry["height"] * metadata["scale"] * factor
        crop = sheet.crop((round(x), round(y), round(x + width), round(y + height)))
        archive = entry["archive"]
        os.makedirs(os.path.dirname(archive), exist_ok=True)
        crop.save(archive)
        if copy_to_pack:
            pack = entry["pack"]
            os.makedirs(os.path.dirname(pack), exist_ok=True)
            crop.save(pack)
    print(f"Recortadas {len(metadata['entries'])} poses en {os.path.join('assets', rom_name, 'ai-remaster', 'poses')}")


def main():
    parser = argparse.ArgumentParser(description="Hoja de poses de Alex para Gemini")
    parser.add_argument("command", choices=("build", "slice"))
    parser.add_argument("rom")
    parser.add_argument("sheet", nargs="?", help="PNG generado por Gemini (solo para slice)")
    parser.add_argument("--pack", action="store_true", help="copiar también a remaster_workspace/objects")
    args = parser.parse_args()
    if args.command == "build":
        build(args.rom)
    else:
        if not args.sheet:
            parser.error("slice necesita el PNG generado por Gemini")
        slice_sheet(args.rom, args.sheet, args.pack)


if __name__ == "__main__":
    main()
