"""Convierte el catalogo de una ROM a HD cinematografico.

Unico comando de remasterizado: usa el motor vectorial (SDF, antialiasing,
bisel y luz) con la paleta de cine aplicada a cada region antes de sombrear.

    make cinematic ROM="roms/juego.sms"
    make cinematic ROM="roms/juego.sms" CINE_MATCH=sprite_
"""

import argparse
import json
import os
import sys
import time

import numpy as np
from PIL import Image

import grades
import vector_tool


def parse_light(value):
    parts = [piece.strip() for piece in value.split(",")]
    if len(parts) != 2:
        raise argparse.ArgumentTypeError("la luz se indica como 'azimut,elevacion', por ejemplo 315,48")
    return float(parts[0]), float(parts[1])


def parse_args():
    parser = argparse.ArgumentParser(
        description="Remasteriza en HD con paleta cinematografica (sin IA).",
    )
    parser.add_argument("--rom-name", required=True, help="Carpeta de assets de la ROM")
    parser.add_argument("--only", help="Procesa solo este ID")
    parser.add_argument("--match", default="", help="Procesa solo los IDs que empiezan por este prefijo")
    parser.add_argument("--limit", type=int, default=0, help="Maximo de composites; 0 procesa todos")
    parser.add_argument("--no-tiles", dest="tiles", action="store_false", default=True,
                        help="No regenera el fallback por tile")
    parser.add_argument("--skip-existing", action="store_true",
                        help="No rehace los PNG que ya existen")
    parser.add_argument("--bevel", type=float, default=1.8, help="Radio del bisel en pixeles de juego")
    parser.add_argument("--light", type=parse_light, default=(315.0, 48.0),
                        help="Luz como azimut,elevacion")
    parser.add_argument("--noise", type=float, default=0.03, help="Amplitud del grano")
    parser.add_argument("--supersample", type=int, default=2,
                        help="Muestras por pixel de juego en la fase de color (1 desactiva)")
    parser.add_argument("--shaded", action="store_true",
                        help="Hornea la luz en el PNG (modo antiguo) en vez de dejar albedo plano")
    parser.add_argument("--no-heights", dest="heights", action="store_false", default=True,
                        help="No genera el mapa de altura para la iluminacion en GPU")
    return parser.parse_args()


def render_file(source, target, args, scale, grade, height_target=None):
    with Image.open(source) as image:
        rgba = np.asarray(image.convert("RGBA"), dtype=np.uint8)
    started = time.monotonic()
    want_height = height_target is not None
    result = vector_tool.render(
        rgba, scale, args.bevel, args.light, args.noise, None, args.supersample, grade,
        flat=not args.shaded, want_height=want_height,
    )
    if want_height:
        image_out, height_out = result
    else:
        image_out = result
    os.makedirs(os.path.dirname(target), exist_ok=True)
    image_out.save(target, format="PNG", optimize=False, compress_level=3)
    if want_height:
        os.makedirs(os.path.dirname(height_target), exist_ok=True)
        height_out.save(height_target, format="PNG", optimize=False, compress_level=3)
    return time.monotonic() - started


def main():
    args = parse_args()
    grade = grades.GRADES["cinematic"]

    root = os.path.join("assets", args.rom_name)
    work = os.path.join(root, "remaster_workspace")
    source_dir = os.path.join(work, "input", "objects")
    object_dir = os.path.join(work, "objects")
    height_object_dir = os.path.join(work, "heights", "objects")
    fallback_dir = os.path.join(work, "tile_fallback")
    height_fallback_dir = os.path.join(work, "heights", "tile_fallback")
    tile_dir = os.path.join(root, "original", "tiles")
    if not os.path.isdir(source_dir):
        sys.exit(f"No existe {source_dir}; ejecuta make unpack primero")

    try:
        scale = int(os.environ.get("GOSMS_SCALE", "4"))
    except ValueError:
        sys.exit("GOSMS_SCALE debe ser un numero entero")
    if scale < 1:
        sys.exit("GOSMS_SCALE debe ser >= 1")

    sources = sorted(name for name in os.listdir(source_dir) if name.lower().endswith(".png"))
    if args.match:
        sources = [name for name in sources if name.startswith(args.match)]
    if args.only:
        sources = [name for name in sources if os.path.splitext(name)[0] == args.only]
        if not sources:
            sys.exit(f"No se encontro {args.only}.png en {source_dir}")
    if args.limit:
        sources = sources[:args.limit]

    # El empaquetador lee esta marca para graduar tambien el fondo del VDP.
    os.makedirs(work, exist_ok=True)
    with open(os.path.join(work, "grade.json"), "w", encoding="utf-8") as output:
        json.dump({"name": "cinematic"}, output)

    print(
        f"Grado cinematografico: paleta de cine, albedo {'plano' if not args.shaded else 'horneado'} "
        f"y alturas {'on' if args.heights else 'off'} a {scale}x "
        f"(bisel {args.bevel}, luz {args.light[0]:.0f},{args.light[1]:.0f}, "
        f"grano {args.noise}, supermuestreo {args.supersample})"
    )
    done = skipped = 0
    total = 0.0
    for name in sources:
        target = os.path.join(object_dir, name)
        height_target = os.path.join(height_object_dir, name) if args.heights else None
        # Con alturas activas un color sin su mapa no sirve: hay que regenerar
        # los dos para que la luz no trate la pieza como una superficie plana.
        complete = os.path.exists(target) and (height_target is None or os.path.exists(height_target))
        if complete and args.skip_existing:
            skipped += 1
            continue
        total += render_file(os.path.join(source_dir, name), target, args, scale, grade, height_target)
        done += 1
    if skipped:
        print(f"   {skipped} objetos ya existian")
    if done:
        print(f"   {done} composites en {total:.1f}s ({total / done * 1000:.0f} ms por objeto)")

    if not args.tiles:
        return
    tile_names = sorted(name for name in os.listdir(tile_dir) if name.lower().endswith(".png")) \
        if os.path.isdir(tile_dir) else []
    if not tile_names:
        print("   No hay original/tiles; se omite el fallback por tile")
        return
    started = time.monotonic()
    written = 0
    for name in tile_names:
        target = os.path.join(fallback_dir, name)
        height_target = os.path.join(height_fallback_dir, name) if args.heights else None
        complete = os.path.exists(target) and (height_target is None or os.path.exists(height_target))
        if complete and args.skip_existing:
            continue
        render_file(os.path.join(tile_dir, name), target, args, scale, grade, height_target)
        written += 1
    print(f"   {written} tiles en {time.monotonic() - started:.1f}s")


if __name__ == "__main__":
    main()
