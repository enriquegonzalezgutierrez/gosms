# Controles de GoSMS

## Escritorio (Linux / Windows)

### Menú

| Tecla | Acción |
| --- | --- |
| `Enter` / `Espacio` | Jugar con la ROM seleccionada |
| Flechas o `A` / `D` | Elegir juego |
| `O` | Cambiar carpeta de ROMs (`roms/` ↔ carpeta actual) |
| `R` | Regenerar la carátula del juego seleccionado |
| `F12` o `H` | Abrir / cerrar la ayuda de controles |
| `F1` | Cambiar filtro de vídeo (incluye Modern+ y Modern+ Parallax) |
| `F2` | Cambiar filtro de audio |
| `F11` | Pantalla completa |
| `Esc` | Salir |

### Menú con mando

| Mando | Acción |
| --- | --- |
| D-pad / stick izquierdo | Elegir juego |
| `A` / `Start` | Jugar |
| `B` / `Back` | Salir |
| A / Start / B / Back | Cerrar la ayuda |

### Juego

| Tecla | Acción |
| --- | --- |
| Flechas | Mover |
| `Z` o `Espacio` | Botón 1 (disparo / salto) |
| `X` | Botón 2 (salto / disparo) |
| `P` o `Enter` | PAUSE / START (en Alex Kidd abre el mapa) |
| `F1` o `V` | Siguiente filtro de vídeo |
| `F2` o `B` | Siguiente filtro de audio |
| `F3` | Guardar partida rápida |
| `F4` | Cargar partida rápida |
| `F5` | Captura de pantalla |
| `F6` o `M` | Música de fondo ON/OFF |
| `F10` | Alternar HD / original (si hay pack) |
| `F11` | Pantalla completa |
| `F12` o `H` | Ayuda de controles (pausa la emulación) |
| `Esc` | Volver al menú |

En el juego, el mando usa D-pad/stick para mover y A/B como botones 1/2.

### Filtros de vídeo

`Píxel perfect`, `CRT scanlines`, `Scale2x EPX`, `Suavizado`, `Fósforo verde`, `REMASTER 2026`, `MODERN+`, `MODERN+ STRONG`, `MODERN+ PARALLAX`.

La música de fondo es opcional: si existe `assets/<ROM>/music/theme.ogg` (Ogg **Vorbis**, no Opus), se reproduce en bucle y silencia los canales de tono del PSG para no duplicar la música original; los efectos de ruido siguen sonando. `F6` o `M` la activa o desactiva. Usa `-music-mix` si quieres escuchar ambos. Si el OGG no es Vorbis, se convierte automáticamente con `ffmpeg`.

### Filtros de audio

`Audio puro`, `Calidez analógica`, `Pseudo-estéreo 3D`, `Salón recreativo`.

## Android

- **ELEGIR ROM / CAMBIAR ROM**: abre el selector de archivos del sistema.
- **JUGAR**: inicia la ROM.
- **D-pad**: mover.
- **1 / 2**: botones de acción.
- **PAUSA**: PAUSE / START del juego.
- **MENÚ**: vuelve al lanzador.
- **Atrás**: desde el juego vuelve al lanzador; desde el lanzador sale.
- **CONTROLES**: abre esta ayuda en el lanzador.

Los mandos USB/Bluetooth se detectan automáticamente y el modelo aparece en el menú.
