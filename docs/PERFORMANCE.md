# Rendimiento y FPS

Este documento fija la línea base de rendimiento **antes** de cualquier refactor
de arquitectura (SOLID, puertos/adaptadores, división de la UI, etc.). La idea es
poder comparar y detectar regresiones sin depender de la percepción visual.

## Qué se mide

Hay tres niveles, de más fino a más realista:

1. **Microbenchmarks de Go** (`make bench`): CPU, VDP, PSG, `Machine.RunFrame`
   y pipeline de vídeo. No dependen de una ROM externa ni de una ventana.
2. **FPS de emulación headless** (`make perf ROM=...`): ejecuta la ROM real sin
   UI ni audio, midiendo CPU + VDP. Es la métrica más estable para comparar
   refactors del núcleo.
3. **FPS real con ventana** (`make bench-ui ROM=...`): incluye Ebiten, escalado,
   filtros, HD y VSync desactivado. Depende de la GPU, del driver y del tamaño de
   ventana; úsalo como referencia local, no como test automático.

## Línea base

Máquina de referencia:

- CPU: Intel Core i5-8265U @ 1.60 GHz (8 hilos)
- SO: Linux
- Go: 1.25.0
- Fecha: 2026-10-29

### `make bench` (una pasada, `-benchtime=1s`)

| Benchmark | ns/op | Métrica | allocs/op |
|---|---:|---|---:|
| `BenchmarkStep` | 61,84 | 16.169.536 instr/s | 0 |
| `BenchmarkStepPorted` | 80,41 | 12.436.931 instr/s | 0 |
| `BenchmarkRenderLine` | 1.265 | 790.451 líneas/s | 0 |
| `BenchmarkRenderFrame` | 227.834 | 4.389 frames/s (0,228 ms/frame) | 0 |
| `BenchmarkRender` (PSG) | 28.466 | 28.139.222 muestras/s | 0 |
| `BenchmarkRunFrame` | 755.037 | 1.324 frames/s (0,755 ms/frame) | 0 |
| `BenchmarkRunCycles` | 7.540 | 132.634.302 ciclos/s | 0 |
| `BenchmarkDraw/pixel` | 4.884 | 204.768 frames/s | 6 |
| `BenchmarkDraw/modern` | 22.754 | 43.948 frames/s | 57 |
| `BenchmarkDraw/crt` | 29.067 | 34.404 frames/s | 65 |
| `BenchmarkDrawHD/modern` | 15.900 | 62.895 frames/s | 47 |

> Los benchmarks de `pkg/video` usan Ebiten. En esta máquina se ejecutan en el
> backend disponible en el entorno de test; no equivalen al coste real de una
> GPU concreta. Para FPS real usa `make bench-ui`.

### `make perf` (Castle of Illusion, 600 frames, warmup 60)

```
bench frames=540 warmup=60 elapsed=1.509s ms_per_frame=2.794 fps=357.9
      cycles_per_frame=60953 instr_per_frame=6166
```

Es decir, el núcleo CPU+VDP puede emular Castle of Illusion a **~358 FPS** en
esta máquina, muy por encima de los 60 FPS necesarios.

### FPS real con UI: HD + Remaster 2026 (la métrica importante)

Medido con **Sonic 1**, **3600 frames de warmup** (para salir de la intro y
llegar a gameplay) y **1200 frames medidos**, con `-hd` (F10) y filtro
`REMASTER 2026` (índice 5):

| HD | Filtro | Escala | Resolución | ms/frame | FPS |
|---|---|---|---:|---:|---:|
| off | Pixel Perfect | 3 | 768×576 | 2,774 | 360,4 |
| off | Remaster 2026 | 3 | 768×576 | 3,479 | 287,4 |
| on | Pixel Perfect | 3 | 768×576 | 5,186 | 192,8 |
| **on** | **Remaster 2026** | **3** | **768×576** | **6,161** | **162,3** |
| **on** | **Remaster 2026** | **5,625** | **1440×1080** | **8,213** | **121,8** |

La combinación **HD + Remaster 2026** es la que hay que vigilar: es la más
costosa y la que usa el usuario final con F10. Aun así, en la máquina de
referencia queda muy por encima de 60 FPS tanto a 768×576 como a 1080p.

> No midas solo 120–180 frames: eso se queda en la intro. Usa al menos
> `BENCH_WARMUP=3600` para que la medición caiga en gameplay real.

## Comandos

```bash
# Microbenchmarks (3 pasadas, con memoria)
make bench

# FPS de emulación headless con una ROM real
make perf ROM="roms/Castle of Illusion Starring Mickey Mouse (U) (V1.1) [!].sms" PERF_FRAMES=600

# FPS real con ventana: por defecto ya mide HD + Remaster 2026
# (3600 frames de warmup y 3600 medidos; súbelo si hace falta)
make bench-ui ROM="roms/Castle of Illusion Starring Mickey Mouse (U) (V1.1) [!].sms"

# Matrix completa: HD off/on × Pixel/Remaster 2026
make bench-ui-matrix ROM="roms/Sonic The Hedgehog (USA, Europe).sms" \
  PERF_FRAMES=1200 BENCH_WARMUP=3600 BENCH_SCALE=3
```

También se puede llamar directamente:

```bash
go test ./pkg/cpu ./pkg/vdp ./pkg/psg ./pkg/machine ./pkg/video \
  -run '^$' -bench . -benchmem -count 3

go run ./cmd/headless -bench -frames 600 -bench-warmup 60 "roms/juego.sms"

# HD + Remaster 2026, 3600 de warmup y 1200 medidos
go run ./cmd/emulator -bench 1200 -bench-warmup 3600 \
  -hd -filter 5 -scale 5.625 "roms/juego.sms"
```

## Cómo comparar antes/después

1. Guarda la salida actual en un fichero:
   ```bash
   make bench > /tmp/bench-antes.txt
   make perf ROM="..." PERF_FRAMES=600 > /tmp/perf-antes.txt
   ```
2. Haz el refactor.
3. Repite y compara:
   ```bash
   make bench > /tmp/bench-despues.txt
   # opcional, si tienes benchstat:
   benchstat /tmp/bench-antes.txt /tmp/bench-despues.txt
   ```
4. Para `make perf`, compara `fps` y `ms_per_frame`; para los microbenchmarks,
   usa `benchstat` o compara medias de 3 pasadas.

## Criterios de aceptación del refactor

- **Núcleo**: `BenchmarkRunFrame` y `BenchmarkStep` no deben empeorar más de un
  **5 %** de media en 3 pasadas. Un 5 % puede ser ruido; si hay duda, repite.
- **VDP/PSG**: sin regresión superior al 5 %.
- **Vídeo**: sin regresión superior al 5 % en `BenchmarkDraw`; para HD, valida
  además con `make bench-ui` en una máquina con GPU.
- **FPS headless**: mantener al menos **300 FPS** con Castle en la máquina de
  referencia (margen amplio sobre 60 FPS).
- **FPS HD + Remaster 2026**: mantener al menos **120 FPS** a 768×576 y
  **90 FPS** a 1440×1080 en la máquina de referencia. El objetivo real es no
  bajar de 60 FPS estables en 1080p.
- **Cero allocs** en `BenchmarkStep`, `BenchmarkRunFrame` y `BenchmarkRenderFrame`.
  Si un refactor introduce allocs en el bucle caliente, se considera regresión.

## Limitaciones

- La ROM sintética de los microbenchmarks no representa todos los juegos; sirve
  para comparar el mismo código antes y después.
- `make bench-ui` depende de la ventana y de la GPU; no lo uses como único
  criterio en CI.
- El backend de Ebiten en tests puede ser software; los números de `pkg/video`
  son útiles para detectar regresiones relativas, no como FPS absolutos.
- El pack HD no se carga en `cmd/headless`; para medir HD real usa la UI.
