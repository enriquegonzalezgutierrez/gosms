# Guía paso a paso: mejorar gráficos de una ROM con IA

Esta guía está escrita para quien no conoce los tiles ni el funcionamiento del emulador. Al terminar, tendrás una imagen modificada dentro del juego y podrás compararla con la original usando **F10**.

> **Importante:** puedes darle al empaquetador un resultado de IA grande, incluso de 4K, y de formato JPEG. En esta primera etapa, GoSMS lo adapta a la cuadrícula de textura 4× que usa actualmente. Esto permite importar una imagen grande sin que se descuadren sus piezas; todavía no convierte el juego ni sus texturas internas a resolución 4K.

## Antes de empezar

Necesitas:

1. GoSMS descargado y sus dependencias de compilación instaladas.
2. La ROM del juego en `roms/`. Para estos ejemplos usaremos `roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms`.
3. Python 3 y Pillow. Desde una terminal abierta en la carpeta del proyecto, instala Pillow una sola vez:

   ```bash
   python3 -m pip install Pillow
   ```

4. Una herramienta de **edición de imágenes con IA (image-to-image)**. Debe permitir subir una imagen existente y descargar el resultado. En la herramienta, utiliza la imagen original como referencia; no crees una imagen solo a partir de texto. PNG con transparencia es lo preferible, pero el empaquetador también acepta JPEG y WebP.

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
5. Descarga el resultado a la resolución más alta que permita la herramienta, hasta 4K si está disponible. El empaquetador acepta PNG, JPEG o WebP. PNG con transparencia es lo preferible; si la herramienta solo ofrece JPEG, el empaquetador recupera la transparencia del original.

### Prompt para copiar

```text
Edit only the single attached image. Identify its existing character or object from the image itself. This is a faithful remaster of the original, not a redesign. Keep the exact same character/object, clothing, costume details, accessories, colors, patterns, pose, facial expression, direction, proportions, silhouette, and placement. Keep every limb and component in exactly the same position. Do not invent details that are not visible in the reference.

Improve only the rendering quality so it looks like a polished 2026 game asset: cleaner contours, clearer shading, richer material detail, and refined lighting. Keep the original art direction and color palette. This image is assembled from game tiles, so keep the same aspect ratio and composition. Do not crop, stretch, rotate, move, add, remove, merge, or split any parts. Do not create a new scene or change the pose.

Preserve the transparent background and all empty areas. Do not add text, numbers, labels, borders, shadows outside the original silhouette, or extra objects. Return one high-resolution image, up to 4K on its longest side if supported, with the same aspect ratio as the input. Do not put multiple versions in one image. Prefer transparent PNG; JPEG or WebP is acceptable if PNG is unavailable.
```

### Comprobar el resultado antes de importarlo

Abre la imagen descargada junto al original y comprueba que:

- el dibujo sigue siendo el mismo personaje u objeto;
- no se ha cortado ninguna parte y no se ha añadido un fondo;
- conserva aproximadamente la misma proporción de ancho y alto y no se ha recortado;
- el contenido sigue en la misma posición y el fondo añadido no tapa el objeto;
- la imagen no está borrosa ni contiene texto o elementos inventados.

La IA puede cambiar el tamaño y eso está permitido. No permitas que recorte o cambie mucho la proporción. Al empaquetar, GoSMS ajusta la imagen completa al tamaño de referencia sin estirarla y después vuelve a colocar los cortes en la cuadrícula de **32×32 píxeles por tile**. Si la proporción cambia mucho, el ajuste dejará márgenes transparentes y el objeto puede quedar demasiado pequeño; genera de nuevo la imagen conservando la proporción.

## Paso 4: importar (guardar) el PNG nuevo en GoSMS

Supongamos que la imagen fuente se llama `sprite_1234567890abcdef.png`. Guarda o copia el resultado de la IA aquí:

```text
assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/ai_workspace/objects/sprite_1234567890abcdef.png
```

La regla es: **misma carpeta `objects/` y exactamente el mismo nombre de archivo** que la imagen de `input/objects/`. Si la carpeta o el archivo no existen, créalos. No sustituyas la imagen original de `input/objects/`; así puedes volver a compararla o generar otra versión.

Repite los pasos 2–4 para cada objeto que quieras cambiar. Si no hay un PNG nuevo con el nombre correspondiente, ese objeto utilizará sus gráficos originales.

### Ejemplo completo con un PNG real de Alex Kidd

Con la ROM de Alex Kidd indicada en esta guía y la extracción predeterminada, se genera un grupo que muestra a Alex. Su imagen fuente se llama `sprite_0600d6d542c2c03e.png` y mide **140×192 píxeles**. Usa estos pasos para probar el flujo con ese archivo. Si el nombre no aparece en tu extracción, elige el PNG que muestre a Alex y sustituye el nombre del ejemplo por el nombre exacto de ese archivo en los pasos 1 y 4.

1. En el explorador de archivos, abre:

   ```text
   assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/ai_workspace/input/objects/sprite_0600d6d542c2c03e.png
   ```

2. Abre la herramienta de IA, elige edición de imagen (*image-to-image*) y carga ese PNG. Copia el prompt genérico de la sección anterior sin añadir el nombre de Alex ni cambiar sus instrucciones.
3. Descarga el resultado en la resolución más alta que ofrezca la herramienta. Por ejemplo, la imagen de la captura mide **864×1220** y es JPEG; se puede importar aunque no tenga el mismo tamaño que el original. Comprueba que Alex conserve la pose y dirección y que la imagen no esté recortada.
4. Guarda el resultado en esta carpeta con el mismo ID. Si es JPEG, la extensión será `.jpg`; si es PNG, usa `.png`:

   ```text
   assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/ai_workspace/objects/sprite_0600d6d542c2c03e.jpg
   ```

   Si la descarga tiene otro nombre, cámbialo para que coincida con el ID indicado. No reemplaces el archivo que está en `input/objects/`: ese es el original de comparación.
5. Desde la carpeta del proyecto, crea el pack y abre el mismo juego:

   ```bash
   make pack ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
   make run ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
   ```

6. Cuando aparezca el juego, pulsa **F10**. Alex debería verse con el nuevo dibujo. Pulsa F10 otra vez para comparar con el original.

Si quieres comprobar el tamaño y formato descargado, ejecuta este comando desde la carpeta del proyecto. En el ejemplo JPEG, mostrará `(864, 1220) RGB` (el tamaño puede variar según la herramienta):

```bash
python3 -c "from PIL import Image; p='assets/Alex Kidd in Miracle World (UE) (V1.1) [!]/ai_workspace/objects/sprite_0600d6d542c2c03e.jpg'; im=Image.open(p); print(im.size, im.mode)"
```

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
