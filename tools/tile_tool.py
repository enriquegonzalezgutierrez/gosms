import csv
import json
import math
import os
import shutil
import sys

import grades
import numpy as np
from PIL import Image, ImageChops, ImageDraw, ImageFont, ImageOps


SCALE = max(1, int(os.environ.get("GOSMS_SCALE", "4")))
CELL = 12 * SCALE
PADDING = 2 * SCALE
TILE_SIZE = 8 * SCALE
PAGE_SIZE = max(1024, int(os.environ.get("GOSMS_PAGE_SIZE", "4096")))
# Large packs are split into texture pages. Small packs keep the compact v1
# single-sheet format unless the caller explicitly asks for pages.
WRITE_PAGES = SCALE >= 8 or os.environ.get("GOSMS_PAGES", "0") == "1"


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


def remastered_object_path(work_dir, object_id):
    # Image generators may return a different format than requested. Prefer
    # lossless formats if more than one result is present.
    for extension in ("png", "webp", "jpg", "jpeg"):
        path = os.path.join(work_dir, "objects", f"{object_id}.{extension}")
        if os.path.isfile(path):
            return path
    return None


def load_remastered_object(path, reference_path, expected_size):
    with Image.open(path) as source:
        generated = source.convert("RGBA")

    if generated.size != expected_size:
        # Fit without cropping or stretching, then place on the manifest-sized
        # canvas. This accepts large AI outputs while preserving the tile grid.
        fitted = ImageOps.contain(generated, expected_size, method=Image.Resampling.LANCZOS)
        canvas = Image.new("RGBA", expected_size, (0, 0, 0, 0))
        position = ((expected_size[0] - fitted.width) // 2,
                    (expected_size[1] - fitted.height) // 2)
        canvas.alpha_composite(fitted, position)
        generated = canvas

    # JPEG has no alpha channel. Reuse the source object's alpha mask so its
    # background stays transparent; intersect it with any alpha from the AI.
    if reference_path and os.path.isfile(reference_path):
        with Image.open(reference_path) as reference:
            reference_alpha = reference.convert("RGBA").getchannel("A")
        if reference_alpha.size != expected_size:
            reference_alpha = reference_alpha.resize(expected_size, Image.Resampling.NEAREST)
        generated.putalpha(ImageChops.multiply(generated.getchannel("A"), reference_alpha))
    return generated


def is_remasterable_object(obj):
    # ROM tile atlases are technical collections of 8x8 patterns, not coherent
    # objects for image-to-image remastering. Keep them in the manifest so pack
    # can map their tiles, but do not present them as AI object inputs.
    return not obj.get("kind", "").endswith("_tileset")


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
    for scene_index, scene in enumerate(scenes):
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
        frame = scene.get("frame")
        if not isinstance(frame, int):
            stem = os.path.splitext(os.path.basename(scene.get("image", "")))[0]
            digits = "".join(ch for ch in stem if ch.isdigit())
            frame = int(digits) if digits else scene_index
        image.save(os.path.join(scene_dir, f"frame_{frame:05d}_tiles.png"))


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


def object_source_path(obj, original_dir):
    preview = obj.get("preview") or os.path.join("objects", f"{obj['id']}.png")
    path = os.path.join(original_dir, preview.replace("/", os.sep))
    return path if os.path.isfile(path) else None


def complete_object_image(obj, original_dir, work_dir, scale):
    """Devuelve la imagen completa de un objeto, remasterizada si existe.

    Los objetos completos (pantallas de fondo y poses de sprites) ya se
    exportan renderizados en original/objects. Si el artista o la IA han
    dejado un resultado en remaster_workspace/objects, se usa ese en su lugar.
    """
    width = obj.get("width", 0)
    height = obj.get("height", 0)
    if width <= 0 or height <= 0:
        return None
    expected = (width * scale, height * scale)
    remastered = remastered_object_path(work_dir, obj["id"]) if work_dir else None
    reference = object_source_path(obj, original_dir)
    if remastered:
        return load_remastered_object(remastered, reference, expected)
    if not reference:
        return None
    # El original se guarda a resolución nativa para no llenar la VRAM: el
    # emulador lo escala al dibujar. Un remaster de la IA sí conserva la
    # resolución esperada (width * scale).
    with Image.open(reference) as source:
        return source.convert("RGBA")


def export_complete_objects(document, original_dir, work_dir, hd_dir, scale):
    """Exporta hd/objects.json y las imágenes completas usadas en runtime.

    Solo se exportan objetos con firma y ancla: son los que el emulador puede
    reconocer como una pantalla o una pose completa. Los objetos técnicos por
    tiles siguen viviendo únicamente en el atlas.
    """
    objects = []
    out_dir = os.path.join(hd_dir, "objects")
    os.makedirs(out_dir, exist_ok=True)
    wanted = set()
    for obj in document.get("objects", []):
        if obj.get("kind") not in ("background_screen", "sprite_cluster", "sprite", "sprite_pose"):
            continue
        if not obj.get("signature") or not obj.get("anchor"):
            continue
        image = complete_object_image(obj, original_dir, work_dir, scale)
        if image is None:
            continue
        name = f"{obj['id']}.png"
        image.save(os.path.join(out_dir, name))
        wanted.add(name)
        objects.append({
            "id": obj["id"],
            "kind": obj["kind"],
            "label": obj.get("label", ""),
            "image": f"objects/{name}",
            "width": obj.get("width", 0),
            "height": obj.get("height", 0),
            "palette": obj.get("palette", ""),
            "anchor": obj["anchor"],
            "signature": obj["signature"],
        })
    for name in os.listdir(out_dir):
        if name.lower().endswith(".png") and name not in wanted:
            os.remove(os.path.join(out_dir, name))
    with open(os.path.join(hd_dir, "objects.json"), "w", encoding="utf-8") as output:
        json.dump({"version": 1, "scale": scale, "objects": objects}, output, indent=2, ensure_ascii=False)
    print(f"🧩 {len(objects)} objeto(s) completo(s) exportados a {out_dir}/ y hd/objects.json")
    return objects


def unpack(rom_name):
    root = os.path.join("assets", rom_name)
    original_dir = os.path.join(root, "original")
    tile_dir = os.path.join(original_dir, "tiles")
    out_dir = os.path.join(root, "remaster_workspace")
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

    remasterable_objects = [obj for obj in document["objects"] if is_remasterable_object(obj)]
    # input/objects is generated from original/manifest.json. Clear old previews
    # first so stale tile-atlas images do not survive a later unpack.
    for name in os.listdir(source_object_dir):
        if name.lower().endswith(".png"):
            os.remove(os.path.join(source_object_dir, name))

    for obj in remasterable_objects:
        placements = obj.get("tiles", [])
        complete = complete_object_image(obj, original_dir, None, SCALE)
        if complete is not None:
            complete.save(os.path.join(source_object_dir, f"{obj['id']}.png"))
            continue
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

    make_object_contact_sheet(remasterable_objects, source_object_dir, out_dir)
    make_scene_tile_maps(document.get("scenes", []), tile_images, out_dir)

    with open(os.path.join(out_dir, "objects.csv"), "w", newline="", encoding="utf-8") as output:
        writer = csv.writer(output)
        writer.writerow(("id", "label", "kind", "frames_seen", "tile_indices", "source_image", "remaster_output"))
        for obj in document["objects"]:
            indices = sorted({tile["index"] for tile in obj.get("tiles", [])})
            is_object = is_remasterable_object(obj)
            writer.writerow((obj["id"], obj.get("label", "unlabeled"), obj.get("kind", ""),
                             obj.get("frames_seen", 0), ";".join(map(str, indices)),
                             f"input/objects/{obj['id']}.png" if is_object else "",
                             f"objects/{obj['id']}.png" if is_object else ""))

    if spritesheet is not None:
        spritesheet.close()

    readme = os.path.join(out_dir, "README.txt")
    with open(readme, "w", encoding="utf-8") as output:
        output.write("Usa input/objects/ para remasterizar objetos coherentes y guarda los resultados en objects/ con el mismo ID; se aceptan PNG, JPEG y WebP a cualquier resolución.\n")
        output.write(f"El empaquetador adapta el resultado, sin recortarlo ni estirarlo, a la cuadrícula original de {TILE_SIZE} px por tile y recupera la transparencia del original.\n")
        output.write("pack aplica primero tile_fallback/, después los objetos y por último los overrides de tiles/ (mayor prioridad).\n")
        output.write("Los sprite_tileset y background_tileset son atlas técnicos de patrones 8x8: no son objetos y no se exportan a input/objects. Sus entradas se conservan en el manifest para mapear sus tiles.\n")
        output.write("Los tiles compartidos por varios objetos generan conflictos; resuélvelos creando un override en tiles/bg_NNNN.png o tiles/spr_NNNN.png.\n")
        output.write("Edita el campo label de objects[] en original/manifest.json para identificar cada objeto.\n")

    print(f"✅ {len(document['tiles'])} tiles en el manifest; {len(remasterable_objects)} composites fuente en {source_object_dir}/ (atlas técnicos omitidos)")
    print("👉 Remasteriza los PNG de input/objects/ y guarda los resultados en objects/ sin cambiar el tamaño.")


def write_palette(hd_dir, work_dir):
    """Guarda el color de los 64 registros CRAM ya graduados.

    El fondo del VDP no es un tile: lo pinta el registro 7 sobre el que se
    dibujan los tiles. Sin este mapa, el cielo conserva el azul puro de la
    consola mientras el resto de la escena usa la paleta de cine.
    """
    grade_name = "none"
    marker = os.path.join(work_dir, "grade.json")
    if os.path.exists(marker):
        try:
            with open(marker, encoding="utf-8") as source:
                grade_name = json.load(source).get("name", "none")
        except (OSError, ValueError):
            grade_name = "none"
    grade = grades.GRADES.get(grade_name)
    colors = {}
    for code in range(64):
        rgb = ((code & 3) * 85, ((code >> 2) & 3) * 85, ((code >> 4) & 3) * 85)
        tinted = grade(rgb) if grade else rgb
        colors["%d,%d,%d" % rgb] = [int(c) for c in tinted[:3]]
    with open(os.path.join(hd_dir, "palette.json"), "w", encoding="utf-8") as output:
        json.dump({"grade": grade_name, "colors": colors}, output, indent=2, ensure_ascii=False)


def grade_name(work_dir):
    marker = os.path.join(work_dir, "grade.json")
    if not os.path.exists(marker):
        return "none"
    try:
        with open(marker, encoding="utf-8") as source:
            return json.load(source).get("name", "none")
    except (OSError, ValueError):
        return "none"


def flatten_sky_heights(remastered, heights, work_dir):
    """Deja la altura del cielo en la referencia plana (0).

    El fondo de las nubes y el cielo comparten el mismo azul graduado. La altura
    de una pieza debe quedar por encima del cielo; si el cielo vale 1.0, el
    borde de la nube es un valle y el shader lo sombrea como una linea negra.
    Con el cielo a 0, las nubes y el terreno quedan por encima y el pase de
    cielo se encarga del degradado.
    """
    name = grade_name(work_dir)
    grade = grades.GRADES.get(name)
    if grade is None:
        return
    sky = np.array(grade((0x00, 0x00, 0xFF))[:3], dtype=np.int16)
    for index, image in remastered.items():
        height = heights.get(index)
        if height is None:
            continue
        rgba = np.asarray(image.convert("RGBA"), dtype=np.int16)
        mask = (np.abs(rgba[..., :3] - sky).max(axis=-1) <= 8) & (rgba[..., 3] > 200)
        if not mask.any():
            continue
        values = np.asarray(height.convert("L"), dtype=np.uint8).copy()
        values[mask] = 0
        heights[index] = Image.fromarray(values, mode="L")


def load_height_map(path, size):
    """Carga el mapa de altura (L) de un tile u objeto, o None si no existe."""
    if not os.path.exists(path):
        return None
    with Image.open(path) as source:
        return source.convert("L").resize(size, Image.Resampling.BILINEAR)


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
    work_dir = os.path.join(root, "remaster_workspace")
    hd_dir = os.path.join(root, "hd")
    os.makedirs(hd_dir, exist_ok=True)
    document = read_manifest(os.path.join(original_dir, "manifest.json"))
    tiles = document["tiles"]
    tiles_by_index = {item["index"]: item for item in tiles}
    original_sheet_path = os.path.join(original_dir, "spritesheet.png")
    original_sheet = Image.open(original_sheet_path).convert("RGBA") if os.path.exists(original_sheet_path) else None

    remastered = {}
    heights = {}
    conflicts = []
    conflict_sources = {}

    # Start with exact nearest-neighbor versions; object composites replace these.
    for item in tiles:
        index = item["index"]
        source = load_original_tile(item, tile_dir, original_sheet)
        if source is not None:
            remastered[index] = source.resize((TILE_SIZE, TILE_SIZE), Image.Resampling.NEAREST)
        heights[index] = None

    # Per-tile neural fallback. It covers tiles that no coherent object reaches,
    # but object composites still take precedence below.
    fallback_dir = os.path.join(work_dir, "tile_fallback")
    fallback_tiles = set()
    for item in tiles:
        index = item["index"]
        fallback = os.path.join(fallback_dir, tile_filename(item))
        if os.path.exists(fallback):
            with Image.open(fallback) as source:
                remastered[index] = source.convert("RGBA").resize((TILE_SIZE, TILE_SIZE), Image.Resampling.LANCZOS)
            fallback_tiles.add(index)
            heights[index] = load_height_map(
                os.path.join(work_dir, "heights", "tile_fallback", tile_filename(item)),
                (TILE_SIZE, TILE_SIZE),
            )
    fallback_count = len(fallback_tiles)

    object_tiles = {}
    object_owners = {}
    # When two objects share a tile, prefer the one seen in more frames. Sprites
    # win ties because a character tile is usually more visible than a piece of
    # background reused across scenes.
    objects_by_priority = sorted(
        document["objects"],
        key=lambda obj: (
            1 if obj.get("kind") == "sprite_cluster" else 0,
            obj.get("frames_seen", 0),
            len(obj.get("tiles", [])),
        ),
        reverse=True,
    )
    for obj in objects_by_priority:
        path = remastered_object_path(work_dir, obj["id"])
        if path is None:
            continue
        placements = obj.get("tiles", [])
        if not placements:
            continue
        logical_width = max(tile["x"] + 8 for tile in placements)
        logical_height = max(tile["y"] + 8 for tile in placements)
        expected_size = (logical_width * SCALE, logical_height * SCALE)
        reference_path = os.path.join(work_dir, "input", "objects", f"{obj['id']}.png")
        with Image.open(path) as source:
            original_size = source.size
        composite = load_remastered_object(path, reference_path, expected_size)
        height_composite = load_height_map(
            os.path.join(work_dir, "heights", "objects", f"{obj['id']}.png"), expected_size,
        )
        if original_size != expected_size:
            print(f"🔎 Adaptando {os.path.basename(path)}: {original_size[0]}x{original_size[1]} → {expected_size[0]}x{expected_size[1]} (cuadrícula {SCALE}x)")
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
                conflict_sources.setdefault(index, []).append(obj["id"])
                continue
            object_tiles[index] = crop
            object_owners[index] = obj["id"]
            if height_composite is not None:
                heights[index] = height_composite.crop((x, y, x + TILE_SIZE, y + TILE_SIZE))
    remastered.update(object_tiles)

    # An explicit per-tile override takes precedence over any grouped object.
    override_dir = os.path.join(work_dir, "tiles")
    override_tiles = set()
    for item in tiles:
        index = item["index"]
        override = os.path.join(override_dir, tile_filename(item))
        if os.path.exists(override):
            with Image.open(override) as source:
                remastered[index] = source.convert("RGBA").resize((TILE_SIZE, TILE_SIZE), Image.Resampling.LANCZOS)
            override_tiles.add(index)
            heights[index] = load_height_map(
                os.path.join(work_dir, "heights", "tiles", tile_filename(item)),
                (TILE_SIZE, TILE_SIZE),
            )

    flatten_sky_heights(remastered, heights, work_dir)

    export_complete_objects(document, original_dir, work_dir, hd_dir, SCALE)

    covered_tiles = fallback_tiles | set(object_tiles) | override_tiles
    nearest_count = max(0, len(tiles) - len(covered_tiles))
    coverage = len(covered_tiles) * 100.0 / max(1, len(tiles))
    has_heights = any(height is not None for height in heights.values())

    if WRITE_PAGES:
        cols_per_page = max(1, PAGE_SIZE // CELL)
        rows_per_page = max(1, PAGE_SIZE // CELL)
        capacity = cols_per_page * rows_per_page
        placed = [(item, remastered[item["index"]]) for item in tiles if item["index"] in remastered]
        page_count = max(1, math.ceil(len(placed) / capacity))
        pages = [Image.new("RGBA", (PAGE_SIZE, PAGE_SIZE), (0, 0, 0, 0)) for _ in range(page_count)]
        height_pages = [Image.new("L", (PAGE_SIZE, PAGE_SIZE), 0) for _ in range(page_count)] if has_heights else None
        page_tiles = []
        page_names = []
        height_page_names = []
        pages_dir = os.path.join(hd_dir, "pages")
        os.makedirs(pages_dir, exist_ok=True)

        for i, (item, tile) in enumerate(placed):
            page_index = i // capacity
            slot = i % capacity
            col = slot % cols_per_page
            row = slot // cols_per_page
            x = col * CELL
            y = row * CELL
            paste_with_gutter(pages[page_index], tile, x, y)
            if height_pages is not None:
                height_tile = heights.get(item["index"])
                if height_tile is not None:
                    paste_with_gutter(height_pages[page_index], height_tile, x, y)
            page_tiles.append({
                "hash": item["hash"],
                "page": page_index,
                "x": x + PADDING,
                "y": y + PADDING,
                "w": TILE_SIZE,
                "h": TILE_SIZE,
                "is_sprite": bool(item.get("is_sprite", False)),
            })

        for page_index, page in enumerate(pages):
            name = f"pages/page_{page_index:03d}.png"
            page.save(os.path.join(hd_dir, name))
            page_names.append({"file": name, "width": PAGE_SIZE, "height": PAGE_SIZE})
            if height_pages is not None:
                hname = f"pages/height_{page_index:03d}.png"
                height_pages[page_index].save(os.path.join(hd_dir, hname))
                height_page_names.append({"file": hname, "width": PAGE_SIZE, "height": PAGE_SIZE})

        pack_document = {
            "version": 2,
            "rom": document.get("rom", rom_name),
            "scale": SCALE,
            "tile_size": 8,
            "pages": page_names,
            "tiles": page_tiles,
            "objects": document.get("objects", []),
            "scenes": document.get("scenes", []),
        }
        if height_page_names:
            pack_document["height_pages"] = height_page_names
        with open(os.path.join(hd_dir, "pack.json"), "w", encoding="utf-8") as output:
            json.dump(pack_document, output, indent=2, ensure_ascii=False)

        # Remove the old single-sheet pack so the runtime does not silently
        # fall back to stale art if a page cannot be loaded.
        write_palette(hd_dir, work_dir)
        stale_sheet = os.path.join(hd_dir, "spritesheet.png")
        if os.path.exists(stale_sheet):
            os.remove(stale_sheet)
        stale_manifest = os.path.join(hd_dir, "manifest.json")
        if os.path.exists(stale_manifest):
            os.remove(stale_manifest)
        print(f"✨ Pack HD v2 listo: {len(placed)}/{len(tiles)} tiles en {page_count} página(s) de {PAGE_SIZE}px, escala {SCALE}x")
        print(f"📊 Cobertura: {len(covered_tiles)}/{len(tiles)} tiles ({coverage:.1f}%): "
              f"{fallback_count} fallback, {len(object_tiles)} objetos, {len(override_tiles)} overrides; {nearest_count} nearest")
    else:
        write_palette(hd_dir, work_dir)
        cols = 32
        rows = max(1, (len(tiles) + cols - 1) // cols)
        sheet = Image.new("RGBA", (cols * CELL, rows * CELL), (0, 0, 0, 0))
        for item in tiles:
            index = item["index"]
            tile = remastered.get(index)
            if tile is None:
                continue
            x = (index % cols) * CELL
            y = (index // cols) * CELL
            paste_with_gutter(sheet, tile, x, y)

        sheet.save(os.path.join(hd_dir, "spritesheet.png"))

        height_path = os.path.join(hd_dir, "heights.png")
        if has_heights:
            height_sheet = Image.new("L", sheet.size, 0)
            for item in tiles:
                height_tile = heights.get(item["index"])
                if height_tile is None:
                    continue
                hx = (item["index"] % cols) * CELL
                hy = (item["index"] // cols) * CELL
                paste_with_gutter(height_sheet, height_tile, hx, hy)
            height_sheet.save(height_path)
        elif os.path.exists(height_path):
            os.remove(height_path)
        with open(os.path.join(hd_dir, "manifest.json"), "w", encoding="utf-8") as output:
            json.dump(document, output, indent=2, ensure_ascii=False)
        print(f"✨ Pack HD v1 listo: {len(remastered)}/{len(tiles)} tiles en {hd_dir}/spritesheet.png (escala {SCALE}x)")
        print(f"📊 Cobertura: {len(covered_tiles)}/{len(tiles)} tiles ({coverage:.1f}%): "
              f"{fallback_count} fallback, {len(object_tiles)} objetos, {len(override_tiles)} overrides; {nearest_count} nearest")

    if original_sheet is not None:
        original_sheet.close()
    if conflicts:
        unique_conflicts = sorted(set(conflicts))
        shown = ", ".join(map(str, unique_conflicts[:20]))
        suffix = " ..." if len(unique_conflicts) > 20 else ""
        print(f"⚠️ {len(unique_conflicts)} tiles compartidos con remasters distintos: {shown}{suffix}")
        print(f"   Se conserva la versión del objeto con más frames vistos. Resuelve casos concretos en {override_dir}/.")
        lost = {}
        for index in unique_conflicts:
            for object_id in conflict_sources.get(index, []):
                lost[object_id] = lost.get(object_id, 0) + 1
        report = {
            "scale": SCALE,
            "tile_size": TILE_SIZE,
            "count": len(unique_conflicts),
            "objects_affected": len(lost),
            "rejected_by_object": dict(sorted(lost.items(), key=lambda item: item[1], reverse=True)),
            "tiles": [
                {
                    "index": index,
                    "file": tile_filename(tiles_by_index[index]) if index in tiles_by_index else None,
                    "kept": object_owners.get(index),
                    "rejected": conflict_sources.get(index, []),
                }
                for index in unique_conflicts
            ],
        }
        report_path = os.path.join(hd_dir, "conflicts.json")
        with open(report_path, "w", encoding="utf-8") as output:
            json.dump(report, output, indent=2, ensure_ascii=False)
        print(f"   Informe con el detalle de cada conflicto: {report_path}")


if __name__ == "__main__":
    if len(sys.argv) < 3 or sys.argv[1] not in ("unpack", "pack", "export-objects"):
        print("Uso: python3 tools/tile_tool.py [unpack|pack|export-objects] 'Nombre_ROM'")
        sys.exit(1)
    if sys.argv[1] == "unpack":
        unpack(sys.argv[2])
    elif sys.argv[1] == "export-objects":
        rom_root = os.path.join("assets", sys.argv[2])
        original_dir = os.path.join(rom_root, "original")
        work_dir = os.path.join(rom_root, "remaster_workspace")
        hd_dir = os.path.join(rom_root, "hd")
        document = read_manifest(os.path.join(original_dir, "manifest.json"))
        export_complete_objects(document, original_dir, work_dir, hd_dir, SCALE)
    else:
        pack(sys.argv[2])
