# Remaster HD sin IA: vectorización y volumen

Esta guía explica cómo GoSMS genera un pack de gráficos en alta definición a partir de una ROM, **sin modelos de IA y sin descargas**. Todo el proceso es geometría: campos de distancia, antialiasing y un mapa de
altura que el emulador ilumina en tiempo real.

Al terminar tendrás el juego con los gráficos remasterizados y podrás comparar con el original usando **F10**.

## Qué hace y qué no hace

Hace:

- elimina el escalón de bloque de cada contorno (el borde deja de ser una escalera);
- suaviza las esquinas con antialiasing real;
- puede convertir cada color plano en una superficie con volumen, bisel y brillo
  (luz de altura opcional, **F6**);
- guarda la altura de cada pieza para que la luz se calcule en pantalla, con
  especular, contraluz y sombra de los sprites;
- sustituye el azul plano del cielo por un degradado con halo de sol;
- rompe la uniformidad de las zonas grandes con un grano suave;
- conserva el dibujo, la pose y la posición exactos; la paleta de la consola
  se traduce a una paleta de cine (pelo castaño, piel tostada, azul acero).

No hace:

- no inventa detalle que no está en el sprite (un iris, poros, una boca dibujada);
- no rediseña personajes ni cambia colores;
- no convierte el juego en una foto.

Es un remaster vectorial: el dibujo original, limpio y con relieve.

## Requisitos

- GoSMS compilado y la ROM en `roms/`.
- Python 3 con Pillow y numpy. La forma más cómoda:

  ```bash
  make setup
  ```

  Crea `.venv-remaster/` con ambas librerías. Si prefieres no usar un entorno, instala `numpy` y `Pillow` en tu Python del sistema.

## Un solo comando

```bash
make remaster ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

Hace, en orden:

1. **Borra** `assets/<ROM>/` por completo.
2. Extrae tiles, mapas y composites (`make objects`).
3. Prepara las imágenes de trabajo (`make unpack`).
4. Remasteriza objetos y tiles en cinematográfico (`make cinematic`).
5. Empaqueta el pack HD (`make pack`).

Al terminar, juega con:

```bash
make run ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

Y compara con **F10**. El paso 4 ya aplica el acabado de cine; los ajustes de
paleta, bisel y luz están más abajo.

> El primer paso borra la carpeta de assets de esa ROM. Como todo se regenera desde la ROM, no se pierde nada que no puedas volver a calcular. Los overrides manuales de `remaster_workspace/tiles/` sí se pierden: cópialos fuera antes de repetir `make remaster`.

## Escala

Por defecto el pack es 4×: cada tile de 8×8 pasa a 32×32.

```bash
make remaster ROM="..." PACK_SCALE=8
```

Hay que generar el pack 8× desde el principio: las alturas se calculan a la
misma escala que el color. 8× produce tiles de 64×64 en páginas de textura
(`pack.json` + `pages/`), con sus páginas de altura (`pages/height_*.png`),
pensado para 1440p y 4K. La carga es perezosa: el emulador solo decodifica las
páginas que necesita.

## Cómo funciona el algoritmo

Cada composite se reduce primero a la **rejilla real de píxeles de juego** (por ejemplo 35×48 para Alex). Trabajar ahí, y no sobre los bloques ampliados, es lo que permite después muestrear a cualquier resolución.

1. **Campo de distancia con signo (SDF)** por cada color de la paleta. Un contorno deja de ser una lista de escalones y pasa a ser una isolínea: la frontera entre distancia positiva y negativa.
2. **Antialiasing por cobertura**: el color se mezcla según `clip(sdf·escala + 0,5, 0, 1)`. En vez de píxeles encendidos o apagados hay cobertura fraccionaria, así que el borde queda limpio a cualquier escala.
3. **Albedo plano**: el color de salida es el color de la paleta (con el grado de cine y el grano), **sin luz horneada**. Así la iluminación se puede cambiar sin volver a generar el pack.
4. **Mapa de altura**: manda la distancia a la silueta de la pieza (volumen exterior) y los bordes de color añaden un relieve suave. Se normaliza por el radio del bisel y se guarda como un PNG en escala de grises paralelo al color, con el mismo layout (`pages/height_XXX.png` en los packs paginados).
5. **Grano**: un ruido de valor suave modula el albedo para que las zonas grandes no queden muertas.

Los tiles sueltos se procesan con el borde replicado, de modo que el bisel no dibuja una línea de rejilla en cada unión.

## Iluminación en el emulador

El pack lleva dos capas: color (albedo) y altura. La luz de altura es
**opcional y viene apagada por defecto**: el HD se ve como albedo plano con el
cielo moderno, que es el modo más limpio y el que evita las costuras de altura.
Pulsa **F6** para activar el volumen. Con la luz activa, el emulador compone las
dos capas con la misma geometría y un shader
(`pkg/video/shaders/heightlight.kage`) reconstruye la normal con el gradiente de
altura:

- **Luz direccional** desde arriba a la izquierda, igual para fondos y sprites;
- **especular** ajustable y **contraluz** en los bordes inclinados;
- **oclusión de grietas**: en el interior de las piezas, donde el gradiente es
  fuerte, se oscurece un poco; el borde exterior no se trata como una grieta
  (si no, cada contorno salía con una línea negra);
- **sombra proyectada** de los sprites sobre el fondo, desplazada hacia abajo a
  la derecha y con penumbra de cuatro muestras. Solo proyectan sombra los
  sprites con píxeles transparentes; un tile opaco no dibuja un rectángulo.

La referencia de la luz es la superficie plana: una zona sin altura conserva su
color exacto en vez de oscurecerse. Eso incluye el fondo del VDP y los tiles en
blanco que el juego usa como cielo.

Al empaquetar, la altura sale del composite cuando cubre el tile (la silueta es
coherente entre sus propios tiles); los tiles sin composite usan su altura
individual. Los píxeles del color del cielo se aplanan a altura 0 para que las
nubes y el terreno queden por encima y su borde no se lea como una grieta.

El pack guarda además `palette.json` con los 64 colores CRAM ya graduados. El
cielo de Alex Kidd no es el registro de fondo: son tiles vacíos que el emulador
pinta con el color 0 de la paleta. Sin ese mapa, el cielo se quedaba en azul
eléctrico mientras el resto de la escena usaba la paleta de cine.

### Cielo moderno

El cielo plano del SMS no se queda en un color liso. El pase de composición
(`pkg/video/shaders/composite.kage`) detecta el color exacto del cielo y lo
sustituye por:

- un **degradado vertical** de azul profundo arriba a azul pálido cerca del
  horizonte;
- un **halo de sol** suave en la dirección de la luz, para que el cielo y el
  relieve compartan la misma fuente;
- **dithering** fino para que el degradado no muestre bandas.

Solo cambia los píxeles que coinciden con el color del cielo: el terreno y los
sprites conservan su color. El cian del fondo de las nubes se traduce al mismo
azul acero en `tools/grades.py`, así que ese fondo también coincide y la nube
deja de verse como un rectángulo sobre el degradado.

Para comparar sin el pase:

```bash
go run ./cmd/preview -frames 900 -filter modern -hd -nosky -out plano.png "roms/juego.sms"
```

El cielo se aplica solo con pack HD y los filtros **REMASTER 2026** o **CRT**;
**SUAVIZADO** deja ver el albedo plano. La luz de altura es independiente de eso:
**F6** la enciende o apaga.

Para probar otra dirección de luz sin tocar código:

```bash
go run ./cmd/preview -frames 900 -filter modern -hd \
  -light "-0.38,-0.72" -out preview.png "roms/juego.sms"
```

`-light` son los dos componentes de la dirección en pantalla (x derecha, y
abajo); `-noheight` desactiva el pase para ver el albedo.

## Ajustes

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `PACK_SCALE` | `4` | Resolución final del pack (4× o 8×). |

Los ajustes de dibujo y de paleta están en la sección siguiente.

Ejemplo para probar un solo objeto y volver a empaquetar:

```bash
make cinematic ROM="..." CINE_ONLY=sprite_0600d6d542c2c03e CINE_TILES=0
make pack ROM="..."
```

## Acabado cinematográfico

`make cinematic` usa el mismo motor, pero traduce la paleta de la consola a una
paleta de cine antes de iluminar: pelo castaño, piel tostada, blanco hueso,
carmesí y azul acero. El dibujo y las sombras se calculan sobre esos colores,
así que el volumen sigue siendo geométrico.

```bash
make cinematic ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
make pack ROM="..."
```

Procesa los 483 composites y los 9156 tiles; tarda unos 10 minutos en un equipo de escritorio. Los
colores que no están en la paleta base reciben un grado fílmico suave (más
saturación, sombras frías y luces cálidas).

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `CINE_MATCH` | vacío | Limita por prefijo de ID: `sprite_` solo personajes, `bg_` solo fondos. |
| `CINE_ONLY` | vacío | Procesa un único ID. |
| `CINE_LIMIT` | `0` | Máximo de composites; `0` procesa todos. |
| `CINE_SKIP` | `0` | `1` conserva los PNG que ya existen. |
| `CINE_TILES` | `1` | `0` no regenera el fallback por tile. |
| `CINE_BEVEL` | `1.8` | Radio del bisel en píxeles de juego. |
| `CINE_LIGHT` | `315,48` | Azimut y elevación de la luz. |
| `CINE_NOISE` | `0.03` | Amplitud del grano. |
| `CINE_SHADED` | `0` | `1` hornea la luz en el PNG en vez de dejar albedo plano. |
| `CINE_HEIGHTS` | `1` | `0` no genera mapa de altura; el emulador vuelve al relieve por gradiente. |

La paleta está en `tools/grades.py`; cambiar un color allí y volver a lanzar el
comando es todo lo necesario para probar otra dirección.

## Carpetas

| Carpeta | Contenido |
| --- | --- |
| `original/` | Extracción de la ROM: tiles, manifiesto y escenas. No la edites. |
| `remaster_workspace/input/objects/` | Composites de partida, uno por objeto. Son la entrada del vectorizador. |
| `remaster_workspace/objects/` | Resultado vectorizado. Aquí puedes poner tus propios PNG. |
| `remaster_workspace/heights/objects/` | Mapa de altura de cada composite, con el mismo tamaño. |
| `remaster_workspace/tile_fallback/` | Resultado por tile, para lo que no cubre ningún composite. |
| `remaster_workspace/heights/tile_fallback/` | Altura de esos tiles sueltos. |
| `remaster_workspace/tiles/` | Overrides manuales de un tile suelto. Tienen prioridad sobre todo lo demás. |
| `remaster_workspace/scenes/` | Vistas de nivel para reconocer dónde aparece cada gráfico. Solo referencia. |
| `hd/` | El pack que consume el emulador: color, alturas y `palette.json` con el fondo graduado. |

## Prioridad al empaquetar

1. Overrides manuales en `remaster_workspace/tiles/`.
2. Composites de `remaster_workspace/objects/`.
3. Fallback por tile en `remaster_workspace/tile_fallback/`.
4. Nearest del tile original, solo si nada de lo anterior lo cubre.

Si dos objetos comparten un tile con resultados distintos, el empaquetador conserva la versión del que se vio en más fotogramas (los sprites ganan los empates) e informa del conflicto:

- resumen en consola;
- detalle en `hd/conflicts.json`, con el objeto que gana, los descartados y el nombre del archivo de override que tendrías que crear.

## Bloques de fondo

`make remaster` agrupa el fondo visible en bloques de 8×8 tiles (64×64 píxeles) y los guarda como `bg_...`. Vectorizar un bloque completo es mucho mejor que vectorizar tiles aislados, porque el SDF ve los bordes reales y no inventa contornos en cada unión.

- `BG_GROUP=8` es el valor por defecto. `BG_GROUP=0` desactiva los bloques.
- `BG_PROTECT=2` protege las dos primeras filas de la name table (HUD).

## Comprobar el resultado

```bash
make preview ROM="..." PREVIEW_FILTER=modern PREVIEW_HD=true
```

O dentro del juego:

- **F10** alterna original y remaster.
- **F6** enciende o apaga la luz de altura (útil si el equipo va justo).
- **F1** recorre los filtros de post-proceso. **REMASTER 2026** es el más
  completo (luz, cielo y bloom); **SUAVIZADO** es el más ligero.
- **P** o **Enter** pulsa PAUSE/START (en Alex Kidd abre el mapa e inventario).

Para capturar esa pantalla sin abrir la ventana, `cmd/preview` acepta `-pause`
con el frame en el que quieres pulsar PAUSE:

```bash
go run ./cmd/preview -frames 900 -pause 600 -filter modern -hd \
  -out mapa.png "roms/juego.sms"
```

## Límites conocidos

- Un sprite de 35×48 no contiene un rostro: el resultado será limpio y con volumen, pero no fotorrealista.
- Las esquinas muy agudas se redondean ligeramente; `CINE_BEVEL=0.8` reduce el efecto.
- La luz de altura es un gradiente 2.5D y es opcional: no hay oclusión entre
  objetos distintos ni sombras proyectadas del escenario, solo la sombra suave
  de los sprites cuando está activa. El modo por defecto (albedo plano + cielo)
  es el más limpio y el más rápido.
- Un pack antiguo sin `heights.png`/`palette.json` sigue funcionando, pero usa
  el relieve por gradiente y el fondo sin graduar.
- El texto y el HUD no se vectorizan aparte: si aparecen en un composite, se suavizan como cualquier otra región.
- Los packs 8× ocupan bastante más en disco y en memoria de vídeo.
