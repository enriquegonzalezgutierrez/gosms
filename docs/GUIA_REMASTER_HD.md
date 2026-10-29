# Remaster HD sin IA: vectorización y cielo moderno

Esta guía explica cómo GoSMS genera un pack de gráficos en alta definición a partir de una ROM, **sin modelos de IA y sin descargas**. Todo el proceso es geometría: campos de distancia, antialiasing y una paleta de
cine. El emulador añade el cielo moderno y el postprocesado.

Al terminar tendrás el juego con los gráficos remasterizados y podrás comparar con el original usando **F10**.

## Qué hace y qué no hace

Hace:

- elimina el escalón de bloque de cada contorno (el borde deja de ser una escalera);
- suaviza las esquinas con antialiasing real;
- sustituye el azul plano del cielo por un degradado con halo de sol;
- rompe la uniformidad de las zonas grandes con un grano suave;
- conserva el dibujo, la pose y la posición exactos; la paleta de la consola
  se traduce a una paleta de cine (pelo castaño, piel tostada, azul acero).

No hace:

- no inventa detalle que no está en el sprite (un iris, poros, una boca dibujada);
- no rediseña personajes ni cambia colores;
- no convierte el juego en una foto.

Es un remaster vectorial: el dibujo original limpio, con la paleta de cine y el
cielo moderno.

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
paleta y grano están más abajo.

### Otras ROMs

El perfil de bot por defecto (`BOT=alex-kidd`) está pensado para Alex Kidd. Para
Sonic u otros juegos usa `BOT=default`, que corre a la derecha y salta cada
pocos frames. Para recorrer todos los niveles de los juegos con selector propio:

```bash
# Sonic 1: los 18 actos
make remaster ROM="roms/Sonic The Hedgehog (USA, Europe).sms" BOT=default SONIC_ALL=1

# Castle of Illusion: los 6 mundos
make remaster ROM="roms/Castle of Illusion Starring Mickey Mouse (U) (V1.1) [!].sms" BOT=default CASTLE_ALL=1
```

Sin esos modificadores, la extracción solo ve las escenas por las que pasa el
bot. `FRAMES=6500` cubre el recorrido de cada nivel; para más zonas sube
`FRAMES` o llega antes con level select/save states. En juegos que no sean Alex
Kidd Rev 1 no hay decodificación estática del layout: todo sale de las escenas
capturadas.

> El primer paso borra la carpeta de assets de esa ROM. Como todo se regenera desde la ROM, no se pierde nada que no puedas volver a calcular. Los overrides manuales de `remaster_workspace/tiles/` sí se pierden: cópialos fuera antes de repetir `make remaster`.

## Escala

Por defecto el pack es 4×: cada tile de 8×8 pasa a 32×32.

```bash
make remaster ROM="..." PACK_SCALE=8
```

8× produce tiles de 64×64 en páginas de textura (`pack.json` + `pages/`),
pensado para 1440p y 4K. La carga es perezosa: el emulador solo decodifica las
páginas que necesita.

## Cómo funciona el algoritmo

Cada composite se reduce primero a la **rejilla real de píxeles de juego** (por ejemplo 35×48 para Alex). Trabajar ahí, y no sobre los bloques ampliados, es lo que permite después muestrear a cualquier resolución.

1. **Campo de distancia con signo (SDF)** por cada color de la paleta. Un contorno deja de ser una lista de escalones y pasa a ser una isolínea: la frontera entre distancia positiva y negativa.
2. **Antialiasing por cobertura**: el color se mezcla según `clip(sdf·escala + 0,5, 0, 1)`. En vez de píxeles encendidos o apagados hay cobertura fraccionaria, así que el borde queda limpio a cualquier escala.
3. **Albedo plano**: el color de salida es el color de la paleta (con el grado de cine y el grano), sin luz horneada. El emulador le añade el cielo y el postprocesado.
4. **Grano**: un ruido de valor suave modula el albedo para que las zonas grandes no queden muertas.



## Cielo y paleta

El pack guarda `palette.json` con los 64 colores CRAM ya graduados. El cielo de
Alex Kidd no es el registro de fondo: son tiles vacíos que el emulador pinta con
el color 0 de la paleta. Sin ese mapa, el cielo se quedaba en azul eléctrico
mientras el resto de la escena usaba la paleta de cine.

### Cielo moderno

El cielo plano del SMS no se queda en un color liso. El pase de composición
(`pkg/video/shaders/composite.kage`) detecta el color exacto del cielo y lo
sustituye por:

- un **degradado vertical** de azul profundo arriba a azul pálido cerca del
  horizonte;
- un **halo de sol** suave en la dirección de la luz, coherente con el resto
  de la escena;
- **dithering** fino para que el degradado no muestre bandas.

Solo cambia los píxeles que coinciden con el color del cielo: el terreno y los
sprites conservan su color. El cian del fondo de las nubes se traduce al mismo
azul acero en `tools/grades.py`, así que ese fondo también coincide y la nube
deja de verse como un rectángulo sobre el degradado.

Para comparar sin el pase:

```bash
go run ./cmd/preview -frames 900 -filter modern -hd -nosky -out plano.png "roms/juego.sms"
```

El cielo se aplica solo con pack HD y los filtros **REMASTER 2026**,
**MODERN+**, **MODERN+ STRONG** o **CRT**; **SUAVIZADO** deja ver el albedo
plano.

## Ajustes

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `PACK_SCALE` | `4` | Resolución final del pack (4× o 8×). |
| `PACK_STYLE` | `cinematic` | Estilo del pack: `cinematic`, `modern-plus` o `modern-strong`. |
| `HD_STYLE` | `modern-plus` | Estilo que activa `make select-hd` si hay varios packs generados. |
| `BOT` | `alex-kidd` | Perfil de la extracción; `default` corre a la derecha y salta (Sonic y otros). |
| `SONIC_ALL` | vacío | `1` recorre los 18 actos de Sonic 1 con el selector de nivel por RAM. |
| `CASTLE_ALL` | vacío | `1` recorre los 6 mundos de Castle of Illusion entrando por las puertas. |
| `FRAMES` | `6500` | Frames emulados durante la extracción; más frames, más escenas. |

Los ajustes de dibujo y de paleta están en la sección siguiente.

Ejemplo para probar un solo objeto y volver a empaquetar:

```bash
make cinematic ROM="..." CINE_ONLY=sprite_0600d6d542c2c03e CINE_TILES=0
make pack ROM="..."
```

## Acabado cinematográfico

`make cinematic` usa el mismo motor, pero traduce la paleta de la consola a una
paleta de cine antes de aplicar el color: pelo castaño, piel tostada, blanco
hueso, carmesí y azul acero.

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
| `CINE_NOISE` | `0.03` | Amplitud del grano. |

La paleta está en `tools/grades.py`; cambiar un color allí y volver a lanzar el
comando es todo lo necesario para probar otra dirección.

## Carpetas

| Carpeta | Contenido |
| --- | --- |
| `original/` | Extracción de la ROM: tiles, manifiesto y escenas. No la edites. |
| `remaster_workspace/input/objects/` | Composites de partida, uno por objeto. Son la entrada del vectorizador. |
| `remaster_workspace/objects/` | Resultado vectorizado. Aquí puedes poner tus propios PNG. |
| `remaster_workspace/tile_fallback/` | Resultado por tile, para lo que no cubre ningún composite. |
| `remaster_workspace/tiles/` | Overrides manuales de un tile suelto. Tienen prioridad sobre todo lo demás. |
| `remaster_workspace/scenes/` | Vistas de nivel para reconocer dónde aparece cada gráfico. Solo referencia. |
| `hd/` | El pack que consume el emulador: color y `palette.json` con el fondo graduado. |

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
- **F12** o **H** abre la ayuda de controles (también el tutorial de primer arranque).
- **F1** recorre los filtros de post-proceso. **REMASTER 2026** es el más
  completo (cielo y bloom); **MODERN+** y **MODERN+ STRONG** suben bloom,
  saturación y contraste; **SUAVIZADO** es el más ligero.
- **P** o **Enter** pulsa PAUSE/START (en Alex Kidd abre el mapa e inventario).
- Al conectar un mando USB/Bluetooth, el menú muestra el modelo y el juego avisa
  con un OSD al conectar o desconectar.
- En el menú, **D-pad/stick** elige, **A/Start** juega y **B/Back/Esc** sale; el
  tutorial se cierra con esos mismos botones.

Para capturar esa pantalla sin abrir la ventana, `cmd/preview` acepta `-pause`
con el frame en el que quieres pulsar PAUSE:

```bash
go run ./cmd/preview -frames 900 -pause 600 -filter modern -hd \
  -out mapa.png "roms/juego.sms"
```

## Límites conocidos

- Un sprite de 35×48 no contiene un rostro: el resultado será limpio, pero no fotorrealista.
- El arte es plano: no hay luz volumétrica ni sombras proyectadas; el relieve
  se sustituye por la limpieza del vectorizado y el cielo moderno.
- Un pack antiguo sin `palette.json` sigue funcionando, pero el fondo queda sin
  graduar.
- El texto y el HUD no se vectorizan aparte: si aparecen en un composite, se suavizan como cualquier otra región.
- Los packs 8× ocupan bastante más en disco y en memoria de vídeo.
- La cobertura del pack son las escenas que alcanza el bot de extracción; las
  zonas no visitadas siguen usando los gráficos originales hasta que las
  captures con `FRAMES`/level select.
