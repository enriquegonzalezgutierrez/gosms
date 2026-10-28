# Remaster HD sin IA: vectorización y volumen

Esta guía explica cómo GoSMS genera un pack de gráficos en alta definición a partir de una ROM, **sin modelos de IA y sin descargas**. Todo el proceso es geometría: campos de distancia, antialiasing y una luz fija.

Al terminar tendrás el juego con los gráficos remasterizados y podrás comparar con el original usando **F10**.

## Qué hace y qué no hace

Hace:

- elimina el escalón de bloque de cada contorno (el borde deja de ser una escalera);
- suaviza las esquinas con antialiasing real;
- convierte cada color plano en una superficie con volumen, bisel y brillo;
- rompe la uniformidad de las zonas grandes con un grano suave;
- conserva la paleta, el dibujo, la pose y la posición exactos.

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

Y compara con **F10**. Para el acabado de cine, cambia el paso de vectorizado
por `make cinematic ROM="..."` (ver más abajo).

> El primer paso borra la carpeta de assets de esa ROM. Como todo se regenera desde la ROM, no se pierde nada que no puedas volver a calcular. Los overrides manuales de `remaster_workspace/tiles/` sí se pierden: cópialos fuera antes de repetir `make remaster`.

## Escala

Por defecto el pack es 4×: cada tile de 8×8 pasa a 32×32.

```bash
make pack ROM="..." PACK_SCALE=8
```

8× genera tiles de 64×64 en páginas de textura (`pack.json` + `pages/`), pensado para 1440p y 4K. La carga es perezosa: el emulador solo decodifica las páginas que necesita.

## Cómo funciona el algoritmo

Cada composite se reduce primero a la **rejilla real de píxeles de juego** (por ejemplo 35×48 para Alex). Trabajar ahí, y no sobre los bloques ampliados, es lo que permite después muestrear a cualquier resolución.

1. **Campo de distancia con signo (SDF)** por cada color de la paleta. Un contorno deja de ser una lista de escalones y pasa a ser una isolínea: la frontera entre distancia positiva y negativa.
2. **Antialiasing por cobertura**: el color se mezcla según `clip(sdf·escala + 0,5, 0, 1)`. En vez de píxeles encendidos o apagados hay cobertura fraccionaria, así que el borde queda limpio a cualquier escala.
3. **Volumen**: la misma SDF da la altura de la región. Su gradiente produce un campo de normales y se ilumina con Lambert + especular + oclusión aproximada, con una luz fija arriba a la izquierda.
4. **Grano**: un ruido de valor suave modula el albedo para que las zonas grandes no queden muertas.

Los tiles sueltos se procesan con el borde replicado, de modo que el bisel no dibuja una línea de rejilla en cada unión.

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

Procesa los 483 composites y los 9156 tiles; tarda alrededor de 15 minutos. Los
colores que no están en la paleta base reciben un grado fílmico suave (más
saturación, sombras frías y luces cálidas).

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `CINE_MATCH` | vacío | Limita por prefijo de ID: `sprite_` solo personajes, `bg_` solo fondos. |
| `CINE_ONLY` | vacío | Procesa un único ID. |
| `CINE_LIMIT` | `0` | Máximo de composites; `0` procesa todos. |
| `CINE_SKIP` | `0` | `1` conserva los PNG que ya existen. |
| `CINE_TILES` | `1` | `0` no regenera el fallback por tile. |
| `CINE_BEVEL` | `0.45` | Radio del bisel en píxeles de juego. |
| `CINE_LIGHT` | `315,48` | Azimut y elevación de la luz. |
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
| `hd/` | El pack que consume el emulador. |

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
- **F1** recorre los filtros de post-proceso.

## Límites conocidos

- Un sprite de 35×48 no contiene un rostro: el resultado será limpio y con volumen, pero no fotorrealista.
- Las esquinas muy agudas se redondean ligeramente; `CINE_BEVEL=0` reduce el efecto.
- El texto y el HUD no se vectorizan aparte: si aparecen en un composite, se suavizan como cualquier otra región.
- Los packs 8× ocupan bastante más en disco y en memoria de vídeo.
