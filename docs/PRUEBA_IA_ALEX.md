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

```bash
.venv-remaster/bin/python tools/apply_object.py \
  "assets/Alex Kidd in Miracle World (UE) (V1.1) [!]" \
  bg_14b93b286febf2ee \
  dist/ai-test/salida/bloque-alex-01.png
```

La herramienta:

1. Adapta la imagen al lienzo del objeto (64×64 lógicos a 4× = 256×256).
2. La recorta con la silueta original.
3. La trocea en los 8 tiles únicos del bloque.
4. Escribe esos tiles en `hd/spritesheet.png`.
5. La primera vez guarda `hd/spritesheet.png.bak`.

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

- El bloque aislado queda muy bien: piedra con grano, biseles, oclusión
  ambiental y hierba con volumen. Es el look cinematográfico que se buscaba.
- En el nivel **mezcla dos estilos**: el bloque nuevo es beige con juntas
  negras y hierba realista; el resto del escenario sigue siendo marrón con
  contorno vectorial. Se ven transiciones duras y líneas negras.
- El bloque original reutiliza **8 tiles en 64 posiciones**. La imagen de
  Gemini no es periódica, así que 56 posiciones tienen contenido distinto al
  tile que les toca. La herramienta usa la primera aparición y avisa:
  `56 posiciones comparten tile con otra distinta`.
- Consecuencia: el bloque se repite con el patrón de 8 tiles, no con el dibujo
  completo de Gemini. Para que sea fiel habría que cambiar el mapa de tiles o
  generar una imagen exactamente periódica.

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

- Parche aplicado en `assets/Alex Kidd.../hd/spritesheet.png`.
- Copia limpia en `hd/spritesheet.png.bak`.
- Herramienta y este documento en Git.
