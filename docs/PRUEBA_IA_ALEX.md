# Prueba de remaster con IA — Alex Kidd

Registro de la prueba hecha con Gemini para ver si una imagen generada puede
entrar en el pack HD. **No forma parte del pipeline**: el remaster normal sigue
siendo geométrico y determinista.

Fecha: 30 de octubre de 2026.

## Objetivo y alcance

- Solo **Alex Kidd in Miracle World**.
- Objeto de prueba: `bg_14b93b286febf2ee`, bloque de suelo con hierba de
  **64×64 píxeles de juego** (8 tiles únicos reutilizados en 64 posiciones).
- La escena completa se descartó: el modelo no respeta la rejilla de 8×8 y no
  se puede trocear en tiles.

## Ficheros

| Ruta | Contenido |
| --- | --- |
| `dist/ai-test/entrada/bloque-alex-512.png` | Entrada que se sube a Gemini (bloque a 8×). |
| `dist/ai-test/salida/bloque-alex-01.png` | Resultado de Gemini (1024×1024). |
| `dist/ai-test/comparacion/bloque-alex-hd-actual.png` | Cómo era el bloque antes. |
| `dist/ai-test/PROMPT.txt` | Copia local de los prompts. |
| `tools/apply_object.py` | Herramienta que mete la imagen en el pack. |

`dist/` está ignorado por Git. Este documento es el registro permanente.

## Prompt que funcionó (bloque)

```text
Vas a remasterizar un bloque de fondo de un juego de Sega Master System de 8 bits para un pack HD cinematográfico de 2026.

La imagen es un bloque de 64x64 píxeles de juego ampliado a 512x512: cada píxel de juego mide 8x8. Es un bloque que se repite en el nivel, así que sus bordes tienen que casar con los bloques vecinos.

Devuelve SOLO ese bloque, en un lienzo cuadrado de 512x512, con exactamente la misma composición.

NO es un redibujado vectorial plano. Quiero materiales con volumen y luz:
- Piedra: grano, biseles suaves, oclusión ambiental en las juntas y un brillo especular muy sutil.
- Hierba: volumen, sombra propia entre briznas y un rim light tenue en el borde superior.
- Grading cinematográfico: sombras ligeramente frías, luces cálidas y contraste suave.
- Iluminación global suave desde arriba a la izquierda, pero SIN sombras proyectadas sobre el bloque ni degradados direccionales que rompan el tileado.

PROHIBIDO:
- Estilo vector plano, clipart, mobile game o cel-shading simple.
- Redibujar o simplificar las formas: mismas piedras, misma hierba, mismas posiciones.
- Añadir o quitar piedras, grietas, plantas, flores o musgo.
- Fondo, marco, borde, texto, marca de agua o cuadrícula.
- Pintura al óleo, 3D con perspectiva o desenfoque global.
- Cambiar el encuadre o recortar.

El resultado se cortará en tiles de 8x8 y se repetirá; si cambias el patrón, aparecerán costuras.
```

### Prompt de escena (solo referencia visual)

No se puede usar en el juego, pero sirve para decidir el look:

```text
Remasteriza esta captura como un fotograma de un remake cinematográfico de 2026. No quiero un redibujado vectorial plano.

- Misma composición, mismos objetos y mismas posiciones.
- Iluminación cinematográfica: luz cálida desde arriba a la izquierda, sombras suaves, oclusión ambiental, rim light y profundidad atmosférica.
- Materiales: piedra con grano, hierba con volumen, tela con pliegues.
- Grading: sombras frías, luces cálidas, contraste suave.
- Conserva la paleta original y el HUD legible.

PROHIBIDO: vector plano, clipart, mobile game, simplificar formas, mover objetos, añadir elementos, texto, marcos, marcas de agua o cambiar el encuadre.
```

### Qué falló en el primer intento

El primer prompt pedía “remaster cinematográfico” sin prohibir el vector plano.
Gemini devolvió un redibujado plano tipo juego móvil: sin materiales, sin
volumen, con formas simplificadas y detalles nuevos. La clave fue añadir
**“NO es un redibujado vectorial plano”** y describir materiales, AO y rim
light.

## Cómo preparar la entrada

```bash
.venv-remaster/bin/python - <<'PY'
from PIL import Image
src = "assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/original/objects/bg_14b93b286febf2ee.png"
Image.open(src).convert("RGB").resize((512, 512), Image.Resampling.NEAREST).save(
    "dist/ai-test/entrada/bloque-alex-512.png")
PY
```

## Cómo aplicarlo al juego

### Objeto completo (recomendado)

Dibuja la imagen de Gemini **entera**, sin trocearla, allí donde aparece el
patrón de tiles del objeto. Es la única forma de conservar el detalle de la
imagen original cuando el objeto reutiliza tiles.

```bash
.venv-remaster/bin/python tools/apply_object.py \
  "assets/Alex Kidd in Miracle World (UE) (V1.1) [!]" \
  bg_14b93b286febf2ee \
  dist/ai-test/salida/bloque-alex-01.png --overlay
```

La herramienta:

1. Adapta la imagen al lienzo del objeto (64×64 lógicos a 4× = 256×256).
2. La recorta con la silueta original.
3. La guarda en `hd/overlays/<id>.png`.
4. Añade a `hd/overlays.json` el patrón de 64 celdas (posición + hash del tile).
5. No toca `hd/spritesheet.png`.

El emulador busca ese patrón en el name table; cuando coincide, dibuja la
imagen completa y se salta los tiles de esas celdas. Si el patrón no aparece,
no cambia nada.

Para quitarlo:

```bash
.venv-remaster/bin/python tools/apply_object.py \
  "assets/Alex Kidd in Miracle World (UE) (V1.1) [!]" \
  bg_14b93b286febf2ee --remove
```

### Método por tiles (solo si el objeto no reutiliza tiles)

```bash
.venv-remaster/bin/python tools/apply_object.py \
  "assets/Alex Kidd in Miracle World (UE) (V1.1) [!]" \
  bg_14b93b286febf2ee \
  dist/ai-test/salida/bloque-alex-01.png
```

1. Adapta la imagen al lienzo del objeto.
2. La recorta con la silueta original.
3. La trocea en los 8 tiles únicos del bloque.
4. Escribe esos tiles en `hd/spritesheet.png`.
5. La primera vez guarda `hd/spritesheet.png.bak`.

Con un bloque que reutiliza tiles, este método pierde el detalle: el juego solo
puede mostrar 8 piezas, no las 64 posiciones de la imagen.

## Cómo verlo

```bash
go run ./cmd/preview -frames 3330 -bot alex-kidd -filter remaster -hd \
  -width 1024 -height 768 -out /tmp/bloque.png \
  "roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

O directamente con `make run`.

## Cómo revertir

```bash
cp "assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/hd/spritesheet.png.bak" \
   "assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/hd/spritesheet.png"
```

## Resultado observado

- La imagen de Gemini está perfecta: piedra con grano, biseles, oclusión
  ambiental y hierba con volumen.
- El **método por tiles** la estropea: el bloque usa 8 tiles reutilizados en 64
  posiciones, así que el juego solo puede mostrar un mosaico de 8 piezas. La
  comparación `bloque-mosaico-vs-ia.png` lo deja claro.
- El **overlay** dibuja la imagen completa y el bloque se ve exactamente como
  Gemini (`bloque-alex-overlay.png`). Ese es el método bueno.
- Queda el choque de estilo con el resto del nivel: el bloque nuevo es beige
  con juntas negras y el escenario sigue siendo marrón. Un solo objeto no puede
  cambiar el nivel entero.
- El parche por tiles se revirtió antes de aplicar el overlay: `spritesheet.png`
  volvió a su estado original y el bloque se dibuja solo desde `overlays/`.

## Periodicidad (para una segunda iteración)

El bloque no es un dibujo libre: sus 8 tiles se reutilizan en 64 posiciones.

- El **interior** se repite exactamente cada **16×16 píxeles de juego** (2×2
  tiles).
- La **franja superior de 16 píxeles** es distinta: tiene 4 tiles especiales
  para el borde de la hierba.

Si Gemini no respeta esa repetición, 56 posiciones acaban usando el tile de
otra posición. Para la próxima prueba, añade al prompt:

```text
El patrón interior se repite exactamente cada 16x16 píxeles de juego (2x2 tiles)
y la franja superior de 16 píxeles es distinta. Mantén esa periodicidad.
```

## Limitaciones y normas

- La IA no sustituye al pipeline: el pack geométrico está completo, alineado y
  sin costuras.
- Para que un pack entero fuera coherente, **todos** los objetos tendrían que
  salir del mismo modelo y estilo; un objeto aislado se nota.
- No ejecutar `make pack`, `make remaster`, `make objects` ni `make select-hd`
  después de aplicar el parche: reconstruyen el pack y lo pierden.
- `hd-cinematic` y `hd-modern-plus` no se tocan.
- Los archivos de `dist/ai-test/` son locales y no se suben a Git.

## Estado

- Overlay aplicado: `hd/overlays/bg_14b93b286febf2ee.png` + `hd/overlays.json`.
- `hd/spritesheet.png` está limpio (el parche por tiles se revirtió).
- `hd-cinematic` y `hd-modern-plus` intactos.
- Herramienta, soporte de overlays en el renderer y este documento en Git.
