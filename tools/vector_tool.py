"""Remasteriza sprites y tiles con geometría, sin modelos ni inferencia.

El pixel art no es una foto reducida: es un dibujo rasterizado con bloques
duros. Lo que se percibe como "pixelado" no es informacion perdida, es un
artefacto de rasterizacion, y se corrige con geometria:

1. Cada color de la paleta se convierte en una region y se calcula su campo de
   distancia con signo (SDF). La silueta deja de ser una escalera y pasa a ser
   una isolinea que se puede muestrear a cualquier resolucion.
2. Los bordes se dibujan con cobertura fraccionaria (antialiasing por SDF), asi
   que desaparecen los escalones.
3. La SDF da tambien la altura de cada region. Con su gradiente se obtiene un
   campo de normales y se ilumina con Lambert + especular. El color plano deja
   de ser plano y pasa a tener volumen.
4. Un ruido suave modula el albedo para romper la uniformidad de las áreas
   grandes.

Todo es determinista y se ejecuta en milisegundos por objeto.
"""

import math

import numpy as np
from PIL import Image

import grades

GAME_TILE = 8


# --------------------------------------------------------------------------- #
# Campos de distancia
# --------------------------------------------------------------------------- #

def _edt_1d(grid):
    """Distancia cuadratica 1D exacta (Felzenszwalb & Huttenlocher)."""
    size = grid.shape[0]
    result = np.zeros(size, dtype=np.float64)
    sites = np.zeros(size, dtype=np.int32)
    bounds = np.zeros(size + 1, dtype=np.float64)
    k = 0
    sites[0] = 0
    bounds[0] = -np.inf
    bounds[1] = np.inf
    for q in range(1, size):
        s = ((grid[q] + q * q) - (grid[sites[k]] + sites[k] * sites[k])) / (2.0 * q - 2.0 * sites[k])
        while s <= bounds[k]:
            k -= 1
            s = ((grid[q] + q * q) - (grid[sites[k]] + sites[k] * sites[k])) / (2.0 * q - 2.0 * sites[k])
        k += 1
        sites[k] = q
        bounds[k] = s
        bounds[k + 1] = np.inf
    k = 0
    for q in range(size):
        while bounds[k + 1] < q:
            k += 1
        delta = q - sites[k]
        result[q] = delta * delta + grid[sites[k]]
    return result


def _edt_2d(mask):
    """Distancia euclidea (en pixeles) de cada punto al pixel True mas cercano."""
    field = np.where(mask, 0.0, 1e12)
    for axis in (0, 1):
        field = np.apply_along_axis(_edt_1d, axis, field)
    return np.sqrt(field)


def signed_distance(mask):
    """SDF en pixeles: positivo dentro de la mascara, negativo fuera.

    ``_edt_2d(mask)`` mide la distancia al pixel True mas cercano, que dentro
    vale 0; ``_edt_2d(~mask)`` mide la distancia al hueco mas cercano, que dentro
    es la profundidad. La resta da la distancia con signo.
    """
    return _edt_2d(~mask) - _edt_2d(mask)


# --------------------------------------------------------------------------- #
# Entrada
# --------------------------------------------------------------------------- #

def detect_block_size(rgba):
    """Mayor n para el que la imagen es exactamente un escalado nearest de n."""
    height, width = rgba.shape[:2]
    for n in range(64, 1, -1):
        if height % n or width % n:
            continue
        small = rgba[::n, ::n]
        if np.array_equal(np.repeat(np.repeat(small, n, axis=0), n, axis=1), rgba):
            return n
    return 1


def load_logical(path):
    """Carga el PNG y lo reduce a la rejilla real de pixeles de juego."""
    with Image.open(path) as source:
        rgba = np.asarray(source.convert("RGBA"), dtype=np.uint8)
    block = detect_block_size(rgba)
    return rgba[::block, ::block], block


def palette_of(logical):
    opaque = logical[logical[..., 3] > 0]
    if not len(opaque):
        return []
    packed = opaque.view(np.uint32).reshape(-1)
    values, counts = np.unique(packed, return_counts=True)
    order = np.argsort(counts)[::-1]
    colors = []
    for index in order:
        rgba = np.frombuffer(values[index].tobytes(), dtype=np.uint8)
        colors.append(tuple(int(c) for c in rgba))
    return colors


# --------------------------------------------------------------------------- #
# Render
# --------------------------------------------------------------------------- #

def resize_float(array, size, resampling=Image.Resampling.BILINEAR):
    image = Image.fromarray(array.astype(np.float32), mode="F")
    return np.asarray(image.resize(size, resampling), dtype=np.float32)


def resize_field(array, size):
    """Remuestrea un campo de distancia conservando los rasgos de un pixel.

    Bilineal interpola los cuatro centros de pixel y el maximo del campo cae en
    las esquinas: una linea de un pixel de ancho se pellizca en el centro y el
    detalle (una estrella, una aspa, un ojo) se lava. Bicubico sobrepasa el
    valor de los centros, asi que el interior del rasgo se mantiene solido y la
    linea sobrevive al remuestreo. El sobrepaso se queda dentro del campo de
    color, nunca en la silueta.
    """
    return resize_float(array, size, Image.Resampling.BICUBIC)


def value_noise(logical_size, seed):
    """Ruido de valor suave: una muestra por pixel de juego, interpolada."""
    rng = np.random.default_rng(seed)
    width, height = logical_size
    grid = rng.random((max(2, height), max(2, width))).astype(np.float32)
    return grid


def render(rgba, scale, bevel, light, noise, pad_edges, supersample=2, grade=None,
           flat=False, want_height=False):
    """Devuelve la imagen remasterizada a ``scale`` x la rejilla logica."""
    block = detect_block_size(rgba)
    logical = rgba[::block, ::block]
    height, width = logical.shape[:2]
    out_size = (width * scale, height * scale)

    opaque = logical[..., 3] > 0
    if pad_edges is None:
        # Un tile de fondo es opaco: sus bordes son "continua igual", no un
        # contorno. Rellenar con el propio borde evita que el SDF dibuje una
        # linea de rejilla en cada union de tiles.
        pad_edges = bool(opaque.mean() > 0.99)
    pad = 2 if pad_edges else 0
    padded = np.pad(logical, ((pad, pad), (pad, pad), (0, 0)), mode="edge") if pad else logical

    colors = palette_of(logical)
    if not colors:
        blank = Image.new("RGBA", out_size, (0, 0, 0, 0))
        return (blank, Image.new("L", out_size, 0)) if want_height else blank
    if len(colors) > 24:
        colors = colors[:24]

    # --- distancia con signo por color, en pixeles de juego ------------------
    distances = []
    for color in colors:
        mask = np.all(padded == np.array(color, dtype=np.uint8), axis=-1)
        distance = signed_distance(mask)
        if pad:
            distance = distance[pad:-pad, pad:-pad]
        distances.append(distance)

    # --- color ---------------------------------------------------------------
    # Una region de un solo pixel de juego es tan ancha como el antialiasing, y
    # en el punto medio su SDF empata con la del vecino: el rasgo se lava a gris.
    # Evaluar la mezcla a doble resolucion y reducir por caja conserva el color
    # central y deja el borde suave.
    step = supersample
    while step > 1 and len(distances) * (out_size[1] * step) * (out_size[0] * step) > 40_000_000:
        step //= 2
    hi_size = (out_size[0] * step, out_size[1] * step)
    hi_fields = np.empty((len(distances), hi_size[1], hi_size[0]), dtype=np.float32)
    for index, distance in enumerate(distances):
        hi_fields[index] = resize_field(distance, hi_size)

    best = hi_fields.argmax(axis=0)
    first_hi = hi_fields.max(axis=0)
    masked = hi_fields.copy()
    np.put_along_axis(masked, best[np.newaxis], -1e6, axis=0)
    second_index = masked.argmax(axis=0)
    second_hi = masked.max(axis=0)
    del masked

    tints = [grade(color) if grade else color for color in colors]
    palette = np.array([tint[:3] for tint in tints], dtype=np.float32) / 255.0
    weight = np.clip((first_hi - second_hi) * (scale * step) + 0.5, 0.0, 1.0)[..., np.newaxis]
    albedo_hi = palette[best] * weight + palette[second_index] * (1.0 - weight)
    if step > 1:
        albedo = albedo_hi.reshape(out_size[1], step, out_size[0], step, 3).mean(axis=(1, 3))
    else:
        albedo = albedo_hi
    del albedo_hi, hi_fields

    # --- silueta -------------------------------------------------------------
    silhouette = signed_distance(opaque if not pad else np.pad(opaque, pad, mode="edge"))
    if pad:
        silhouette = silhouette[pad:-pad, pad:-pad]
    silhouette = resize_float(silhouette, out_size)
    coverage = np.clip(silhouette * scale + 0.5, 0.0, 1.0)

    # --- volumen -------------------------------------------------------------
    first = resize_float(np.max(np.stack(distances), axis=0), out_size)
    height_field = np.clip(first, 0.0, max(0.05, bevel)) * scale
    gradient_y, gradient_x = np.gradient(height_field)
    strength = max(0.2, bevel * scale)
    normal = np.dstack((-gradient_x, -gradient_y, np.full_like(height_field, 1.0 / strength)))
    length = np.linalg.norm(normal, axis=-1, keepdims=True)
    normal = normal / np.maximum(length, 1e-6)

    azimuth = math.radians(light[0])
    elevation = math.radians(light[1])
    light_dir = np.array(
        [-math.cos(azimuth) * math.cos(elevation),
         -math.sin(azimuth) * math.cos(elevation),
         math.sin(elevation)],
        dtype=np.float32,
    )
    diffuse = np.clip((normal * light_dir).sum(axis=-1), 0.0, 1.0)
    # El rango se centra en 1.0 para que la iluminacion module el color original
    # sin oscurecerlo: por debajo de eso el sprite se ve sucio y apagado.
    shade = 0.82 + 0.28 * diffuse

    # Oclusion muy leve: si pesa demasiado dibuja una linea oscura en cada
    # frontera de color y el dibujo parece un sticker con contorno negro.
    relief = np.clip(np.clip(first, 0.0, max(0.05, bevel)) / max(0.05, bevel), 0.0, 1.0)
    ambient_occlusion = 0.92 + 0.08 * relief

    halfway = light_dir + np.array([0.0, 0.0, 1.0], dtype=np.float32)
    halfway /= np.linalg.norm(halfway)
    specular = np.clip((normal * halfway).sum(axis=-1), 0.0, 1.0) ** 26 * 0.10
    rim = (1.0 - np.clip(normal[..., 2], 0.0, 1.0)) ** 3 * 0.05

    if noise > 0:
        grain = resize_float(value_noise((width, height), 1234), out_size)[..., np.newaxis]
        albedo = albedo * (1.0 + noise * (grain - 0.5) * 2.0)

    if flat:
        # Sin luz horneada: el color es albedo puro y toda la iluminacion se
        # calcula en el shader a partir del mapa de altura.
        rgb = np.clip(albedo, 0.0, 1.0) * 255.0
    else:
        rgb = albedo * (shade * ambient_occlusion)[..., np.newaxis]
        rgb = rgb + specular[..., np.newaxis] + rim[..., np.newaxis]
        rgb = np.clip(rgb, 0.0, 1.0) * 255.0

    out = np.dstack((rgb, coverage * 255.0)).astype(np.uint8)
    image = Image.fromarray(out, mode="RGBA")
    if not want_height:
        return image

    # Altura normalizada: 255 en el interior de una region y 0 en su borde, con
    # el bisel como radio. Es lo que convierte el dibujo plano en volumen.
    span = max(0.05, bevel) * scale
    height_norm = np.clip(first / span, 0.0, 1.0) * coverage
    height_img = Image.fromarray((height_norm * 255.0).astype(np.uint8), mode="L")
    return image, height_img
