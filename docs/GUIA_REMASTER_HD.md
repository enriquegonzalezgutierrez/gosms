# Guía paso a paso: mejorar gráficos de una ROM con IA

Esta guía está escrita para quien no conoce los tiles ni el funcionamiento del emulador. Al terminar, tendrás una imagen modificada dentro del juego y podrás compararla con la original usando **F10**.

> **Importante:** el proyecto llama “HD 4×” a las texturas que tienen cuatro veces más píxeles por lado que un gráfico original. No significa que el juego se convierta automáticamente en vídeo 4K (3840×2160).

## Antes de empezar

Necesitas:

1. GoSMS descargado y sus dependencias de compilación instaladas.
2. La ROM del juego en `roms/`. Para estos ejemplos usaremos `roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms`.
3. Python 3 y Pillow. Desde una terminal abierta en la carpeta del proyecto, instala Pillow una sola vez:

   ```bash
   python3 -m pip install Pillow
   ```

4. Una herramienta de **edición de imágenes con IA (image-to-image)**. Debe permitir subir una imagen existente y descargar el resultado como PNG. En la herramienta, utiliza la imagen original como referencia; no crees una imagen solo a partir de texto.

## Paso 1: extraer los gráficos

Abre una terminal en la carpeta de GoSMS y copia este comando:

```bash
make remaster ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

Espera a que termine. Se crearán carpetas dentro de `assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/`.

**Este comando borra primero la carpeta de assets de esa ROM.** Si ya hiciste imágenes con IA que quieras conservar, cópialas fuera de `assets/` antes de ejecutarlo. Después de la primera extracción, no vuelvas a usar `make remaster` para empaquetar cambios: usa `make pack` (paso 5).

Si la extracción ya existe y solo necesitas volver a crear las imágenes de referencia, usa `make unpack`:

```bash
make unpack ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

`make unpack` conserva los resultados de IA en `ai_workspace/objects/` y los overrides en `ai_workspace/tiles/`, pero vuelve a generar las imágenes fuente dentro de `ai_workspace/input/objects/`.

## Paso 2: encontrar una imagen que sí debas editar

Abre esta carpeta con el explorador de archivos:

```text
assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/ai_workspace/input/objects/
```

Abre los PNG como imágenes normales. También puedes mirar `ai_workspace/objects_contact_sheet.png`, que muestra una hoja de contacto con nombres y vistas pequeñas. El nombre del PNG es importante: cópialo exactamente, incluidos los números y la extensión `.png`.

La extracción automática agrupa piezas gráficas cercanas, pero no siempre sabe qué representan. Algunas pueden tener nombres largos como `sprite_...` o representar un grupo incompleto. El archivo `ai_workspace/objects.csv` contiene la etiqueta y el tipo de cada entrada. Si no reconoces una imagen, déjala sin modificar y el juego conservará su versión original.

### Qué carpetas usar

| Carpeta | Para qué sirve |
| --- | --- |
| `ai_workspace/input/objects/` | Imágenes originales que puedes enviar a la IA como referencia. No guardes aquí el resultado. |
| `ai_workspace/objects/` | Aquí se importa el PNG que devolvió la IA, con el mismo nombre que el original. |
| `ai_workspace/tiles/` | Cambios avanzados de un tile individual, por ejemplo `spr_0123.png`. No hace falta tocarlo para empezar. |
| `ai_workspace/scenes/` | Vistas de niveles que ayudan a reconocer dónde aparece un gráfico. Son referencias, no se importan al juego. |
| `original/` | Datos de extracción. No borres ni renombres sus archivos. |

No edites imágenes de atlas técnico como si fueran un personaje u objeto. Las entradas llamadas `sprite_tileset` o `background_tileset` son colecciones de piezas cuadradas de 8×8 píxeles; por eso pueden verse fragmentadas, con colores raros o parecer números. No son una imagen completa. No aparecen en `input/objects/`. Para empezar, modifica solo un objeto reconocible de esa carpeta.

## Paso 3: pedir a la IA que mejore el objeto

En la herramienta de imagen:

1. Elige la función que permite editar o transformar una imagen ya existente (*image-to-image*).
2. Sube **un** PNG desde `input/objects/`.
3. Copia y pega el prompt de abajo sin modificarlo. La IA debe identificar el contenido a partir de la imagen que adjuntaste.
4. Pide una sola imagen de salida. No pidas una hoja de personajes nueva ni varias propuestas dentro de un mismo PNG.
5. Descarga el resultado como **PNG con transparencia**. No uses JPG: JPG no conserva las zonas transparentes.

### Prompt para copiar

```text
Edit the attached image. Identify its existing character, object, or game-art content from the image itself. Improve its appearance while keeping the same recognizable content, silhouette, pose, direction, proportions, and placement.

This is pixel art arranged on a strict grid. Keep the exact canvas width, height, and alignment. Each original game tile is 8x8 pixels and occupies a 32x32 pixel cell in this image (4x scale). Redraw only the existing artwork as clean, polished pixel art with crisp pixel clusters and deliberate shading. Do not blur, use photorealism, or add smooth anti-aliased edges.

Preserve all transparent areas. Do not add a background. Do not add, remove, move, rotate, merge, or split any parts. Do not add text, numbers, labels, borders, or extra objects. Return exactly one transparent PNG with the same pixel dimensions as the input image.
```

### Comprobar el resultado antes de importarlo

Abre el PNG descargado junto al original y comprueba que:

- el dibujo sigue siendo el mismo personaje u objeto;
- no se ha cortado ninguna parte y no se ha añadido un fondo;
- el lienzo tiene exactamente el mismo ancho y alto que el original;
- las zonas transparentes siguen transparentes;
- la imagen no está borrosa ni contiene texto o elementos inventados.

Si la IA devuelve otro tamaño, otra imagen compuesta o un fondo sólido, no la importes todavía. Vuelve a generarla indicando que respete el lienzo, o usa un editor de imágenes para colocar el dibujo en el lienzo original sin estirarlo ni mover sus piezas. Cada tile ocupa una celda de **32×32 píxeles** dentro de estos PNG; al cambiar la alineación, el empaquetador puede cortar el dibujo por lugares incorrectos.

## Paso 4: importar (guardar) el PNG nuevo en GoSMS

Supongamos que la imagen fuente se llama `sprite_1234567890abcdef.png`. Guarda o copia el resultado de la IA aquí:

```text
assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/ai_workspace/objects/sprite_1234567890abcdef.png
```

La regla es: **misma carpeta `objects/` y exactamente el mismo nombre de archivo** que la imagen de `input/objects/`. Si la carpeta o el archivo no existen, créalos. No sustituyas la imagen original de `input/objects/`; así puedes volver a compararla o generar otra versión.

Repite los pasos 2–4 para cada objeto que quieras cambiar. Si no hay un PNG nuevo con el nombre correspondiente, ese objeto utilizará sus gráficos originales.

## Paso 5: crear el pack HD y abrir el juego

En la terminal, empaqueta los cambios:

```bash
make pack ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

Al terminar, el pack queda en:

```text
assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/hd/
```

Ahora ejecuta la misma ROM:

```bash
make run ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

Cuando el juego esté abierto, pulsa **F10** para alternar entre los gráficos originales y los remasterizados. Púlsalo otra vez para volver a los originales. Cada vez que sustituyas o añadas un PNG en `ai_workspace/objects/` o `ai_workspace/tiles/`, ejecuta de nuevo `make pack` antes de jugar.

## Si algo no funciona

| Qué ves | Qué comprobar |
| --- | --- |
| GoSMS dice que no hay pack HD | Ejecuta `make pack` y asegúrate de abrir exactamente la ROM indicada en ese comando. El nombre de la ROM determina la carpeta del pack. |
| F10 no parece cambiar nada | Pulsa F10 dentro del juego y no en otra ventana. Prueba en una zona donde aparezca el objeto que editaste. |
| El objeto no cambió | Comprueba que el PNG está en `ai_workspace/objects/`, que su nombre coincide letra por letra con el PNG fuente y que ejecutaste `make pack` después de guardarlo. |
| El dibujo aparece cortado o desalineado | Restaura el PNG fuente y vuelve a generar el remaster conservando el mismo tamaño del lienzo y la cuadrícula de 32 píxeles. |
| Aparecen piezas mezcladas o el empaquetador avisa de tiles compartidos | Dos objetos usan el mismo tile con remasters distintos. Crea un override individual en `ai_workspace/tiles/` con el nombre indicado por el mensaje de `make pack`, y vuelve a empaquetar. |

## Términos en palabras sencillas

- **Tile:** una pieza cuadrada de 8×8 píxeles que el juego combina para dibujar fondos, personajes y objetos.
- **Composite:** una imagen más grande formada al juntar los tiles de un objeto. Es lo que editas en `input/objects/`.
- **Transparencia (canal alfa):** las zonas invisibles alrededor de un personaje. Deben seguir invisibles para que se vea el escenario detrás.
- **Pack HD:** el conjunto de imágenes preparado por `make pack` que GoSMS puede mostrar al pulsar F10.
