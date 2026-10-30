#!/usr/bin/env python3
"""Aplica una imagen externa a un objeto del pack HD ya empaquetado.

Uso:
    python tools/apply_object.py "assets/<ROM>" <object_id> imagen.png

La imagen se adapta al lienzo del objeto, se recorta con la silueta original
(los píxeles transparentes de los sprites siguen siendo transparentes) y se
trocea en tiles de 8x8 que se escriben directamente en hd/spritesheet.png.
No hace falta regenerar el workspace ni volver a vectorizar el resto del pack.

Ejemplo:
    python tools/apply_object.py "assets/Castle of Illusion ..." \
        bg_829741033466e0d9 ~/Descargas/bloque-gemini.png

El pack se modifica en el sitio. La primera vez se guarda una copia en
hd/spritesheet.png.bak para poder volver atrás.
"""

import argparse
import json
import os
import shutil
import sys

import numpy as np
from PIL import Image, ImageChops, ImageOps


def load_json(path):
    with open(path, encoding="utf-8") as source:
        return json.load(source)


def reference_alpha(asset_dir, obj, width, height):
    """Silueta del objeto: une el alfa de cada tile en su posicion."""
    manifest = load_json(os.path.join(asset_dir, "original", "manifest.json"))
    by_index = {item["index"]: item for item in manifest["tiles"]}
    sheet_path = os.path.join(asset_dir, "original", "spritesheet.png")
    sheet = Image.open(sheet_path).convert("RGBA") if os.path.exists(sheet_path) else None
    cols = sheet.width // 12 if sheet is not None else 0
    mask = Image.new("L", (width, height), 0)
    for placement in obj.get("tiles", []):
        index = placement["index"]
        item = by_index.get(index)
        if item is None:
            continue
        prefix = "spr" if item.get("is_sprite") else "bg"
        tile_path = os.path.join(asset_dir, "original", "tiles", f"{prefix}_{index:04d}.png")
        if os.path.exists(tile_path):
            tile = Image.open(tile_path).convert("RGBA")
        elif sheet is not None:
            ox = (index % cols) * 12 + 2
            oy = (index // cols) * 12 + 2
            tile = sheet.crop((ox, oy, ox + 8, oy + 8))
        else:
            continue
        mask.paste(tile.getchannel("A"), (placement["x"], placement["y"]))
    if sheet is not None:
        sheet.close()
    return mask


def paste_with_gutter(sheet, tile, x, y, tile_size, padding):
    sheet.paste(tile, (x + padding, y + padding))
    sheet.paste(tile.crop((0, 0, 1, tile_size)).resize((padding, tile_size)), (x, y + padding))
    sheet.paste(tile.crop((tile_size - 1, 0, tile_size, tile_size)).resize((padding, tile_size)),
               (x + padding + tile_size, y + padding))
    sheet.paste(tile.crop((0, 0, tile_size, 1)).resize((tile_size, padding)), (x + padding, y))
    sheet.paste(tile.crop((0, tile_size - 1, tile_size, tile_size)).resize((tile_size, padding)),
               (x + padding, y + padding + tile_size))
    for sx, sy, px, py in ((0, 0, 0, 0), (tile_size - 1, 0, padding + tile_size, 0),
                           (0, tile_size - 1, 0, padding + tile_size),
                           (tile_size - 1, tile_size - 1, padding + tile_size, padding + tile_size)):
        sheet.paste(tile.crop((sx, sy, sx + 1, sy + 1)).resize((padding, padding)), (x + px, y + py))


def load_overlays(hd_dir, scale):
    path = os.path.join(hd_dir, "overlays.json")
    if os.path.exists(path):
        document = load_json(path)
    else:
        document = {"version": 1, "scale": scale, "overlays": []}
    document.setdefault("version", 1)
    document.setdefault("scale", scale)
    document.setdefault("overlays", [])
    return document


def save_overlays(hd_dir, document):
    with open(os.path.join(hd_dir, "overlays.json"), "w", encoding="utf-8") as output:
        json.dump(document, output, indent=2, ensure_ascii=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("asset_dir", help="carpeta assets/<ROM>")
    parser.add_argument("object_id", help="id del objeto, por ejemplo bg_14b93b286febf2ee")
    parser.add_argument("image", nargs="?", help="PNG con el resultado (Gemini, editor, etc.)")
    parser.add_argument("--no-backup", action="store_true", help="no crear spritesheet.png.bak")
    parser.add_argument("--overlay", action="store_true",
                        help="dibujar el objeto como una sola imagen, sin trocearlo en tiles")
    parser.add_argument("--remove", action="store_true", help="quitar el overlay del objeto")
    args = parser.parse_args()

    hd_dir = os.path.join(args.asset_dir, "hd")
    manifest = load_json(os.path.join(args.asset_dir, "original", "manifest.json"))
    obj = next((item for item in manifest.get("objects", []) if item["id"] == args.object_id), None)
    if obj is None:
        sys.exit(f"No existe el objeto {args.object_id} en {args.asset_dir}")
    placements = obj.get("tiles", [])
    if not placements:
        sys.exit("El objeto no tiene tiles")

    conflicts_path = os.path.join(hd_dir, "conflicts.json")
    conflicts = load_json(conflicts_path) if os.path.exists(conflicts_path) else {}
    scale = int(conflicts.get("scale", 4) or 4)
    tile_size = 8 * scale
    cell = 12 * scale
    padding = 2 * scale

    width = max(item["x"] + 8 for item in placements)
    height = max(item["y"] + 8 for item in placements)
    expected = (width * scale, height * scale)

    if args.remove:
        document = load_overlays(hd_dir, scale)
        entry = next((item for item in document["overlays"] if item.get("id") == args.object_id), None)
        if entry is None:
            print("No habia overlay para ese objeto.")
            return
        document["overlays"].remove(entry)
        save_overlays(hd_dir, document)
        image_path = os.path.join(hd_dir, entry.get("image", ""))
        if entry.get("image") and os.path.exists(image_path):
            os.remove(image_path)
        print(f"Overlay eliminado: {args.object_id}")
        return

    if not args.image:
        parser.error("falta la imagen")

    image = Image.open(args.image).convert("RGBA")
    if image.size != expected:
        fitted = ImageOps.contain(image, expected, method=Image.Resampling.LANCZOS)
        canvas = Image.new("RGBA", expected, (0, 0, 0, 0))
        canvas.alpha_composite(fitted, ((expected[0] - fitted.width) // 2,
                                        (expected[1] - fitted.height) // 2))
        image = canvas
        print(f"Imagen adaptada: {expected[0]}x{expected[1]} (original {width}x{height} a {scale}x)")

    alpha = reference_alpha(args.asset_dir, obj, width, height).resize(expected, Image.Resampling.NEAREST)
    image.putalpha(ImageChops.multiply(image.getchannel("A"), alpha))

    if args.overlay:
        overlay_dir = os.path.join(hd_dir, "overlays")
        os.makedirs(overlay_dir, exist_ok=True)
        target = os.path.join(overlay_dir, f"{args.object_id}.png")
        image.save(target)
        document = load_overlays(hd_dir, scale)
        document["overlays"] = [item for item in document["overlays"]
                                if item.get("id") != args.object_id]
        by_index = {item["index"]: item for item in manifest["tiles"]}
        pattern = []
        seen = set()
        for placement in placements:
            dx, dy = placement["x"] // 8, placement["y"] // 8
            if (dx, dy) in seen:
                continue
            seen.add((dx, dy))
            pattern.append({"dx": dx, "dy": dy, "hash": by_index[placement["index"]]["hash"]})
        document["overlays"].append({
            "id": args.object_id,
            "image": f"overlays/{args.object_id}.png",
            "width": width,
            "height": height,
            "pattern": pattern,
        })
        save_overlays(hd_dir, document)
        print(f"Overlay guardado: {target} ({len(pattern)} celdas, {width}x{height})")
        print("Se dibuja como una sola imagen donde coincida el patron; spritesheet.png no se toca.")
        return

    sheet_path = os.path.join(hd_dir, "spritesheet.png")
    if not os.path.exists(sheet_path):
        sys.exit(f"No existe {sheet_path}")
    if not args.no_backup:
        backup = sheet_path + ".bak"
        if not os.path.exists(backup):
            shutil.copy2(sheet_path, backup)
            print(f"Copia de seguridad: {backup}")

    sheet = Image.open(sheet_path).convert("RGBA")
    cols = sheet.width // cell
    written = {}
    conflicts_seen = 0
    for placement in placements:
        index = placement["index"]
        crop = image.crop((placement["x"] * scale, placement["y"] * scale,
                           placement["x"] * scale + tile_size,
                           placement["y"] * scale + tile_size))
        if index in written:
            if written[index].tobytes() != crop.tobytes():
                conflicts_seen += 1
            continue
        written[index] = crop
        x = (index % cols) * cell
        y = (index // cols) * cell
        paste_with_gutter(sheet, crop, x, y, tile_size, padding)
    sheet.save(sheet_path)
    sheet.close()

    height_sheet = os.path.join(hd_dir, "height_sheet.png")
    if os.path.exists(height_sheet):
        print("⚠️  El pack tiene height_sheet.png: el relieve de esos tiles no se ha actualizado.")
    if conflicts_seen:
        print(f"⚠️  {conflicts_seen} posiciones comparten tile con otra distinta; se usa la primera.")
    print(f"Listo: {len(written)} tiles de {args.object_id} escritos en {sheet_path}.")
    print("Abre el juego y comprueba las costuras con los bloques vecinos.")


if __name__ == "__main__":
    main()
