import csv
import json
import math
import os
import shutil
import sys

from PIL import Image, ImageDraw, ImageFont


SCALE = 4
CELL = 12 * SCALE
PADDING = 2 * SCALE
TILE_SIZE = 8 * SCALE


def read_manifest(path):
    with open(path, "r", encoding="utf-8") as source:
        value = json.load(source)
    if isinstance(value, list):  # Legacy flat manifest.
        return {"version": 1, "tiles": value, "objects": []}
    value.setdefault("tiles", [])
    value.setdefault("objects", [])
    return value


def tile_filename(item):
    prefix = "spr" if item.get("is_sprite", False) else "bg"
    return f"{prefix}_{item['index']:04d}.png"


def make_object_contact_sheet(objects, source_object_dir, out_dir):
    if not objects:
        return
    columns = 4
    cell_width, cell_height = 240, 190
    rows = math.ceil(len(objects) / columns)
    sheet = Image.new("RGBA", (columns * cell_width, rows * cell_height), (24, 28, 36, 255))
    draw = ImageDraw.Draw(sheet)
    font = ImageFont.load_default()
    for i, obj in enumerate(objects):
        path = os.path.join(source_object_dir, f"{obj['id']}.png")
        if not os.path.exists(path):
            continue
        with Image.open(path) as source:
            preview = source.convert("RGBA")
        preview.thumbnail((cell_width - 24, cell_height - 48), Image.Resampling.NEAREST)
        x = (i % columns) * cell_width
        y = (i // columns) * cell_height
        px = x + (cell_width - preview.width) // 2
        py = y + 4 + (cell_height - 48 - preview.height) // 2
        sheet.alpha_composite(preview, (px, py))
        label = obj.get("label", "unlabeled")
        draw.text((x + 8, y + cell_height - 40), f"{label} · {obj['id']}", font=font, fill=(255, 255, 255, 255))
        draw.text((x + 8, y + cell_height - 24), f"{obj.get('kind', '')} · {len(obj.get('tiles', []))} tiles", font=font, fill=(170, 190, 215, 255))
    sheet.save(os.path.join(out_dir, "objects_contact_sheet.png"))


def make_scene_tile_maps(scenes, tile_images, out_dir):
    scene_dir = os.path.join(out_dir, "scenes")
    os.makedirs(scene_dir, exist_ok=True)
    font = ImageFont.load_default()
    scale = 3
    for scene in scenes:
        if scene.get("metatile_map"):
            source = os.path.join(os.path.dirname(out_dir), "original", scene.get("image", ""))
            if os.path.isfile(source):
                shutil.copyfile(source, os.path.join(scene_dir, os.path.basename(source)))
            continue
        tile_map = scene.get("tile_map", [])
        if not tile_map:
            continue
        rows, columns = len(tile_map), max(len(row) for row in tile_map)
        image = Image.new("RGBA", (columns * 8, rows * 8), (15, 18, 24, 255))
        for y, row in enumerate(tile_map):
            for x, index in enumerate(row):
                tile = tile_images.get(index)
                if tile is not None:
                    image.alpha_composite(tile, (x * 8, y * 8))
        image = image.resize((image.width * scale, image.height * scale), Image.Resampling.NEAREST)
        draw = ImageDraw.Draw(image)
        for y, row in enumerate(tile_map):
            for x, index in enumerate(row):
                x0, y0 = x * 8 * scale, y * 8 * scale
                draw.rectangle((x0, y0, x0 + 8 * scale - 1, y0 + 8 * scale - 1), outline=(0, 0, 0, 130))
                if index >= 0:
                    draw.text((x0 + 2, y0 + 2), str(index), font=font, fill=(255, 255, 0, 255), stroke_width=1, stroke_fill=(0, 0, 0, 255))
        image.save(os.path.join(scene_dir, f"frame_{scene['frame']:05d}_tiles.png"))


def load_original_tile(item, tile_dir, spritesheet):
    path = os.path.join(tile_dir, tile_filename(item))
    if os.path.exists(path):
        with Image.open(path) as source:
            return source.convert("RGBA")
    if spritesheet is not None:
        index = item["index"]
        x = (index % 32) * 12 + 2
        y = (index // 32) * 12 + 2
        return spritesheet.crop((x, y, x + 8, y + 8)).convert("RGBA")
    return None


def unpack(rom_name):
    root = os.path.join("assets", rom_name)
    original_dir = os.path.join(root, "original")
    tile_dir = os.path.join(original_dir, "tiles")
    out_dir = os.path.join(root, "ai_workspace")
    source_object_dir = os.path.join(out_dir, "input", "objects")
    os.makedirs(source_object_dir, exist_ok=True)
    os.makedirs(os.path.join(out_dir, "objects"), exist_ok=True)
    os.makedirs(os.path.join(out_dir, "tiles"), exist_ok=True)  # Optional per-tile overrides.

    document = read_manifest(os.path.join(original_dir, "manifest.json"))
    sheet_path = os.path.join(original_dir, "spritesheet.png")
    spritesheet = Image.open(sheet_path).convert("RGBA") if os.path.exists(sheet_path) else None
    tile_images = {}
    for item in document["tiles"]:
        tile = load_original_tile(item, tile_dir, spritesheet)
        if tile is not None:
            tile_images[item["index"]] = tile

    for obj in document["objects"]:
        placements = obj.get("tiles", [])
        if not placements:
            continue
        width = max(tile["x"] + 8 for tile in placements)
        height = max(tile["y"] + 8 for tile in placements)
        composite = Image.new("RGBA", (width, height), (0, 0, 0, 0))
        for placement in placements:
            tile = tile_images.get(placement["index"])
            if tile is not None:
                composite.alpha_composite(tile, (placement["x"], placement["y"]))
        composite = composite.resize((width * SCALE, height * SCALE), Image.Resampling.NEAREST)
        composite.save(os.path.join(source_object_dir, f"{obj['id']}.png"))

    make_object_contact_sheet(document["objects"], source_object_dir, out_dir)
    make_scene_tile_maps(document.get("scenes", []), tile_images, out_dir)

    with open(os.path.join(out_dir, "objects.csv"), "w", newline="", encoding="utf-8") as output:
        writer = csv.writer(output)
        writer.writerow(("id", "label", "kind", "frames_seen", "tile_indices", "source_image", "ai_output"))
        for obj in document["objects"]:
            indices = sorted({tile["index"] for tile in obj.get("tiles", [])})
            writer.writerow((obj["id"], obj.get("label", "unlabeled"), obj.get("kind", ""),
                             obj.get("frames_seen", 0), ";".join(map(str, indices)),
                             f"input/objects/{obj['id']}.png", f"objects/{obj['id']}.png"))

    if spritesheet is not None:
        spritesheet.close()

    readme = os.path.join(out_dir, "README.txt")
    with open(readme, "w", encoding="utf-8") as output:
        output.write("Usa input/objects/ como referencia y guarda los remasters en objects/ con el mismo nombre y dimensiones.\n")
        output.write("Conserva la cuadrícula de 32 px por tile; cada composite debe seguir a escala 4x.\n")
        output.write("pack aplica cada tile de objeto al índice indicado en original/manifest.json.\n")
        output.write("Los tiles compartidos por varios objetos generan conflictos; resuélvelos creando un override en tiles/bg_NNNN.png o tiles/spr_NNNN.png.\n")
        output.write("Edita el campo label de objects[] en original/manifest.json para identificar cada objeto.\n")

    print(f"✅ {len(document['tiles'])} tiles en el manifest; {len(document['objects'])} composites fuente en {source_object_dir}/")
    print("👉 Remasteriza los PNG de input/objects/ y guarda los resultados en objects/ sin cambiar el tamaño.")


def paste_with_gutter(sheet, tile, x, y):
    sheet.paste(tile, (x + PADDING, y + PADDING))
    sheet.paste(tile.crop((0, 0, 1, TILE_SIZE)).resize((PADDING, TILE_SIZE)), (x, y + PADDING))
    sheet.paste(tile.crop((TILE_SIZE - 1, 0, TILE_SIZE, TILE_SIZE)).resize((PADDING, TILE_SIZE)), (x + PADDING + TILE_SIZE, y + PADDING))
    sheet.paste(tile.crop((0, 0, TILE_SIZE, 1)).resize((TILE_SIZE, PADDING)), (x + PADDING, y))
    sheet.paste(tile.crop((0, TILE_SIZE - 1, TILE_SIZE, TILE_SIZE)).resize((TILE_SIZE, PADDING)), (x + PADDING, y + PADDING + TILE_SIZE))
    for px, py, sx, sy in ((x, y, 0, 0), (x + PADDING + TILE_SIZE, y, TILE_SIZE - 1, 0),
                           (x, y + PADDING + TILE_SIZE, 0, TILE_SIZE - 1),
                           (x + PADDING + TILE_SIZE, y + PADDING + TILE_SIZE, TILE_SIZE - 1, TILE_SIZE - 1)):
        sheet.paste(tile.crop((sx, sy, sx + 1, sy + 1)).resize((PADDING, PADDING)), (px, py))


def pack(rom_name):
    root = os.path.join("assets", rom_name)
    original_dir = os.path.join(root, "original")
    tile_dir = os.path.join(original_dir, "tiles")
    ai_dir = os.path.join(root, "ai_workspace")
    hd_dir = os.path.join(root, "hd")
    os.makedirs(hd_dir, exist_ok=True)
    document = read_manifest(os.path.join(original_dir, "manifest.json"))
    tiles = document["tiles"]
    original_sheet_path = os.path.join(original_dir, "spritesheet.png")
    original_sheet = Image.open(original_sheet_path).convert("RGBA") if os.path.exists(original_sheet_path) else None

    cols = 32
    rows = max(1, (len(tiles) + cols - 1) // cols)
    sheet = Image.new("RGBA", (cols * CELL, rows * CELL), (0, 0, 0, 0))
    remastered = {}
    conflicts = []

    # Start with exact 4x nearest-neighbor versions; object composites replace these.
    for item in tiles:
        index = item["index"]
        source = load_original_tile(item, tile_dir, original_sheet)
        if source is not None:
            remastered[index] = source.resize((TILE_SIZE, TILE_SIZE), Image.Resampling.NEAREST)

    object_tiles = {}
    for obj in document["objects"]:
        path = os.path.join(ai_dir, "objects", f"{obj['id']}.png")
        if not os.path.exists(path):
            continue
        with Image.open(path) as source:
            composite = source.convert("RGBA")
        for placement in obj.get("tiles", []):
            index = placement["index"]
            x, y = placement["x"] * SCALE, placement["y"] * SCALE
            if x + TILE_SIZE > composite.width or y + TILE_SIZE > composite.height:
                continue
            crop = composite.crop((x, y, x + TILE_SIZE, y + TILE_SIZE))
            if crop.size != (TILE_SIZE, TILE_SIZE):
                continue
            if index in object_tiles and object_tiles[index].tobytes() != crop.tobytes():
                conflicts.append(index)
                continue
            object_tiles[index] = crop
    remastered.update(object_tiles)

    # An explicit per-tile override takes precedence over any grouped object.
    override_dir = os.path.join(ai_dir, "tiles")
    for item in tiles:
        index = item["index"]
        override = os.path.join(override_dir, tile_filename(item))
        if os.path.exists(override):
            with Image.open(override) as source:
                remastered[index] = source.convert("RGBA").resize((TILE_SIZE, TILE_SIZE), Image.Resampling.LANCZOS)

    for item in tiles:
        index = item["index"]
        tile = remastered.get(index)
        if tile is None:
            continue
        x = (index % cols) * CELL
        y = (index // cols) * CELL
        paste_with_gutter(sheet, tile, x, y)

    sheet.save(os.path.join(hd_dir, "spritesheet.png"))
    with open(os.path.join(hd_dir, "manifest.json"), "w", encoding="utf-8") as output:
        json.dump(document, output, indent=2, ensure_ascii=False)
    if original_sheet is not None:
        original_sheet.close()
    print(f"✨ Pack HD listo: {len(remastered)}/{len(tiles)} tiles en {hd_dir}/spritesheet.png")
    if conflicts:
        print("⚠️ Tiles compartidos con remasters distintos: " + ", ".join(map(str, sorted(set(conflicts)))))
        print(f"   Resuélvelos con overrides individuales en {override_dir}/.")


if __name__ == "__main__":
    if len(sys.argv) < 3 or sys.argv[1] not in ("unpack", "pack"):
        print("Uso: python3 tools/tile_tool.py [unpack|pack] 'Nombre_ROM'")
        sys.exit(1)
    if sys.argv[1] == "unpack":
        unpack(sys.argv[2])
    else:
        pack(sys.argv[2])
