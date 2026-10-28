"""Grados de color para el remaster.

Un grado recibe un color (RGB o RGBA) y devuelve el color de salida. Se aplica
al color de cada region antes de iluminar, asi que el dibujo y las sombras se
calculan sobre la paleta nueva.
"""

import numpy as np

# Paleta de la consola -> paleta de cine. Los colores principales del juego.
CINEMATIC = {
    (0x55, 0x55, 0x00): (0x2C, 0x1E, 0x15),  # oliva -> castaño oscuro (pelo)
    (0xFF, 0xAA, 0x00): (0xD2, 0x8A, 0x56),  # naranja -> piel tostada
    (0xFF, 0xFF, 0xFF): (0xE6, 0xDF, 0xCE),  # blanco -> hueso
    (0xFF, 0x00, 0x00): (0xC4, 0x33, 0x2B),  # rojo -> carmesi
    (0xAA, 0x00, 0x00): (0x7E, 0x1C, 0x16),  # granate -> granate profundo
    (0x00, 0x00, 0xFF): (0x34, 0x63, 0x9C),  # azul -> azul acero
    (0x00, 0x00, 0x00): (0x1A, 0x15, 0x12),  # negro -> tinta calida
    # Colores que no estaban en la tabla y salian neón. El cian es el fondo de
    # los tiles de nube: al llevarlo al azul acero, la nube deja de verse como
    # un rectangulo y el degradado del cielo la integra.
    (0x00, 0xFF, 0xFF): (0x34, 0x63, 0x9C),  # cian -> azul acero (nubes)
    (0x00, 0xAA, 0xAA): (0x2E, 0x5A, 0x78),  # turquesa -> azul profundo
    (0xFF, 0xFF, 0x00): (0xD2, 0xB0, 0x62),  # amarillo -> oro
}


def filmic(rgb):
    """Grado suave para colores fuera de la paleta: mas saturacion, sombras
    frias y luces calidas."""
    colour = np.asarray(rgb[:3], dtype=np.float32) / 255.0
    luma = 0.2126 * colour[0] + 0.7152 * colour[1] + 0.0722 * colour[2]
    colour = luma + (colour - luma) * 1.0
    colour = np.clip(colour, 0.0, 1.0) ** 1.06
    warm = np.array([0.012, 0.004, -0.012], dtype=np.float32)
    cool = np.array([-0.008, 0.000, 0.014], dtype=np.float32)
    colour = colour + warm * (luma ** 2) + cool * ((1.0 - luma) ** 2)
    colour = np.clip(colour, 0.0, 1.0)
    return tuple(int(round(channel * 255)) for channel in colour)


def cinematic(rgb):
    """Paleta de cine: traduccion exacta y, si el color no esta en la tabla,
    grado filmico."""
    mapped = CINEMATIC.get(tuple(int(c) for c in rgb[:3]))
    return mapped if mapped else filmic(rgb)


GRADES = {
    "none": None,
    "cinematic": cinematic,
}
