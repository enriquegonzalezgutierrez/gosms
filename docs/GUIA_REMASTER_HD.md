# Guía de remasterización HD con IA

Esta guía explica cómo extraer los gráficos de una ROM, elegir qué imágenes modificar, empaquetar los cambios y comprobarlos en GoSMS con F10. El flujo está integrado en el `Makefile` y usa `tools/tile_tool.py`.

## 1. Extraer una ROM

Instala Pillow si aún no está disponible:

```bash
python3 -m pip install Pillow
```

Para Alex Kidd, ejecuta la extracción y prepara el workspace:

```bash
make remaster ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

Esto crea `assets/<nombre de ROM>/original/` y `assets/<nombre de ROM>/ai_workspace/`. **`make remaster` y `make objects` borran antes toda la carpeta de assets de esa ROM**, incluidos los remasters hechos a mano o con IA. Úsalos para empezar una extracción limpia, nunca para actualizar un workspace que ya contiene trabajo que quieras conservar.

Si ya existe `original/manifest.json` y solo quieres volver a preparar las imágenes fuente, usa:

```bash
make unpack ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

`unpack` regenera los PNG de `ai_workspace/input/objects/`, las escenas, el CSV y las instrucciones. Conserva los PNG que hayas guardado en `ai_workspace/objects/` y los overrides de `ai_workspace/tiles/`.

## 2. Qué imágenes modificar

| Ruta | Qué contiene | Qué hacer |
| --- | --- | --- |
| `ai_workspace/input/objects/` | Composites fuente de objetos y grupos de sprites, a escala 4× | Remasterizar los objetos reconocibles. Mantener nombre, lienzo y posiciones de tiles. |
| `ai_workspace/objects/` | Resultados creados con IA | Guardar aquí cada resultado con el mismo nombre del PNG fuente. |
| `ai_workspace/tiles/` | Overrides por tile, por ejemplo `spr_0123.png` o `bg_0456.png` | Usarlos cuando un tile compartido necesite una apariencia distinta según el objeto. El empaquetador los adapta a 32×32. |
| `ai_workspace/scenes/` | Mapas y vistas de escenas para consultar la composición de los niveles | Sirven de referencia; no se empaquetan como texturas de juego. |
| `original/tiles/` y `original/manifest.json` | Tiles originales y sus índices, hashes, paletas y agrupaciones | No editar ni renombrar los índices. El empaquetador los necesita para asignar las texturas correctas. |

`input/objects/` no incluye objetos de forma automática en todos los casos: los grupos detectados durante la emulación pueden aparecer como `sprite_cluster` y tener etiquetas genéricas. Revisa `original/manifest.json` y `ai_workspace/objects.csv`; cambia el campo `label` por nombres como `Alex`, `pez` o `árbol`. Para componer un objeto de fondo que no se haya detectado, añade una entrada `background_object` a `objects` en el manifest, con cada tile y sus coordenadas, y vuelve a ejecutar `make unpack`.

Las entradas `sprite_tileset` y `background_tileset` describen atlas técnicos de patrones 8×8, no objetos completos. Sus mosaicos pueden parecer fragmentos, números o tener paletas distintas por nivel. Se conservan en el manifest para identificar los tiles, pero se omiten de `input/objects/`; no pidas a la IA que convierta esos atlas en un objeto. Para modificar uno de sus tiles concretos, usa su PNG `spr_NNNN.png` o `bg_NNNN.png` en `ai_workspace/tiles/`.

## 3. Remasterizar con una herramienta de imagen a imagen

Usa una herramienta de **image-to-image** que acepte un PNG como referencia y permita descargar PNG con canal alfa. Evita el modo de generación desde texto sin imagen de entrada: puede inventar piezas y cambiar la silueta. Trabaja un objeto cada vez, compara el resultado con el original y guarda el PNG en `ai_workspace/objects/` con el mismo nombre.

Cada tile de juego mide 8×8 píxeles; en los composites fuente ocupa una celda de **32×32 píxeles** (escala 4×). Conserva el tamaño total, la cuadrícula de 32 píxeles y las posiciones: el empaquetador corta el composite por esas coordenadas. No recortes, reordenes ni añadas márgenes. El fondo transparente debe permanecer transparente donde lo estaba; no conviertas píxeles opacos en transparentes ni rellenes con un color plano los huecos transparentes.

### Prompt en inglés

Adjunta el PNG fuente y reemplaza la descripción entre corchetes:

> Remaster this exact video-game sprite/object from the attached image: **[DESCRIBE THE OBJECT, e.g. Alex Kidd, a palm tree, or a fish]**. Preserve its original silhouette, pose, proportions, direction, recognizable details, and every object's position. This is a sprite sheet composite on a strict 4x pixel-art grid: each original 8x8 game tile occupies exactly 32x32 pixels. Keep the exact canvas dimensions and alignment. Redraw only the existing artwork with clean, polished high-resolution pixel art; use crisp pixel clusters and deliberate shading, with no blur, anti-aliased edges, gradients, or photorealism. Preserve the original transparent areas and alpha channel. Do not add, remove, move, rotate, merge, or split any parts. Do not add text, numbers, labels, borders, shadows outside the original silhouette, or a background. Return one transparent PNG at exactly the same pixel dimensions as the input.

Para un resultado más fiel, pide primero una versión y revisa a tamaño real y ampliada. Si la herramienta cambia las dimensiones, alinea y redimensiona el resultado en un editor de imágenes antes de guardarlo; no cambies el tamaño del lienzo ni la cuadrícula de 32 píxeles.

## 4. Empaquetar y jugar

Después de guardar o cambiar cualquier remaster:

```bash
make pack ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
make run ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

`make pack` crea `assets/<nombre de ROM>/hd/spritesheet.png` y `hd/manifest.json`. Arranca esa misma ROM en GoSMS y pulsa **F10** para alternar entre gráficos originales y el pack HD. F10 solo puede activar el modo si el pack se generó para el mismo nombre de ROM que estás ejecutando. Si ves el aviso de que no hay pack HD, confirma el nombre de ROM y vuelve a ejecutar `make pack`.

El pack usa texturas a escala 4×: cada tile de 8×8 se representa internamente con 32×32 píxeles. La resolución final en pantalla depende del tamaño de la ventana; **“4×” no significa que el juego se convierta automáticamente en una salida 4K de 3840×2160**.

Si el empaquetador informa de tiles compartidos con remasters distintos, crea un override individual con el nombre indicado en `ai_workspace/tiles/` y ejecuta `make pack` otra vez. El override tiene prioridad sobre los composites.
