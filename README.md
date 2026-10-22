# Alex Kidd: emulador de Sega Master System en Go

Emulador de la Sega Master System (SMS) escrito en Go, con frontend gráfico
basado en [Ebiten](https://ebitengine.org/). Se usa como ROM de referencia
`roms/alex_kidd.sms` (*Alex Kidd in Miracle World*, USA/Europa, dump No-Intro
verificado), y `roms/sonic.sms` como ROM de control.

## Estado

| Subsistema | Estado |
| ---------- | ------ |
| CPU Z80 | Documentada (tabla de opcodes completa, prefijos CB/ED/DD/FD, instrucciones de bloque) |
| VDP (modo 4) | 16 KB VRAM, CRAM, registros, IRQ de frame/línea, scroll X/Y, fondo con paletas y flips, sprites 8×8 y 8×16 con zoom y *early clock* |
| Mapper | Bancos Sega en `$FFFD-$FFFF`, RAM de 8 KB con espejos |
| Entrada | Mandos 1 y 2 (activo-bajo) en `$DC/$DD` y `$C0/$C1` |
| Sonido | SN76489 (3 tonos + ruido). El hilo de audio pide las muestras al PSG, así que la producción y el consumo coinciden y no hay deriva ni clics |
| Timing | 228 ciclos por línea × 262 líneas (NTSC, ≈59,92 Hz), render línea a línea |

Pendiente: *scroll-lock* (R0 bits 6/7), modos de 224/240 líneas, y algún
detalle de precisión del PSG (ruido periódico).

## Requisitos

- Go 1.25 o superior (el `go.mod` pide `1.25.0`; el *toolchain* de Go puede
  descargarlo automáticamente).
- `CGO_ENABLED=1` y las bibliotecas de sistema que necesita Ebiten (X11,
  OpenGL/EGL, ALSA/PulseAudio en Linux).

## Uso

```sh
make run            # ventana con roms/alex_kidd.sms
make run-sonic      # ventana con roms/sonic.sms
make build          # binarios en bin/
```

Equivalentes directos:

```sh
go run ./cmd/emulator roms/alex_kidd.sms
go run ./cmd/emulator -scale 4 -fullscreen roms/sonic.sms
```

### Controles

| Acción | Jugador 1 | Jugador 2 |
| ------ | --------- | --------- |
| Direcciones | Flechas | `W` `A` `S` `D` |
| Botón 1 | `Z` o `Espacio` | `Q` |
| Botón 2 | `X` o `Enter` | `E` |

`F11` alterna pantalla completa y `Esc` cierra.

## Herramientas de desarrollo

- `cmd/emulator`: frontend Ebiten (ventana, entrada, audio).
- `cmd/headless`: ejecuta N frames sin ventana y escribe un PNG. Útil para
  capturas y depuración:

  ```sh
  go run ./cmd/headless -frames 900 -start 200 -out frame.png roms/alex_kidd.sms
  ```

  Opciones: `-nt` (volcar la *name table*), `-dump addr:len` (volcar VRAM),
  `-wdump addr:len` (contar escrituras a VRAM), `-wlog addr:len` (registrar la
  secuencia de escrituras), `-watch addr` (vigilar escrituras a una dirección de
  RAM), `-wav out.wav` (volcar el audio), `-nosprites`, `-regtrace`.
- `cmd/tracer`: traza paso a paso la CPU:

  ```sh
  go run ./cmd/tracer -n 20000 roms/alex_kidd.sms
  ```

## Estructura

```
pkg/cpu/      Z80: núcleo, tabla de opcodes, memoria y puertos
pkg/vdp/      VDP de la SMS (modo 4) y render a RGBA
pkg/psg/      SN76489
pkg/machine/  reloj central: une CPU, VDP y PSG por frame
cmd/emulator/ frontend Ebiten
cmd/headless/ captura PNG por línea de comandos
cmd/tracer/   traza de ejecución
```

## Notas

- Las ROM no se incluyen en el repositorio; colócalas en `roms/*.sms`.
- El mapper escribe también la RAM bajo `$FFFD-$FFFF`: varios juegos (entre
  ellos Alex Kidd) releen `$FFFF` para salvar y restaurar el banco desde la
  rutina de interrupción.
