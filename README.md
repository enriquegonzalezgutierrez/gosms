<p align="center">
  <img src="docs/media/hero-banner.svg" alt="GoSMS Hero Banner" width="100%"/>
</p>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version"/></a>
  <img src="https://img.shields.io/badge/Accuracy-99.7%25-d946ef?style=for-the-badge" alt="Accuracy"/>
  <img src="https://img.shields.io/badge/Audio-48kHz%20Zero--Jitter-10b981?style=for-the-badge" alt="Audio"/>
  <img src="https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20Android-f59e0b?style=for-the-badge" alt="Platforms"/>
  <a href="#licencia"><img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge" alt="License"/></a>
</p>

---

## ⚡ Descripción

**GoSMS** es un emulador de **Sega Master System (SMS)** de alto rendimiento, ciclo-exacto y multiplataforma, desarrollado en **Go puro** con aceleración gráfica mediante [Ebitengine](https://ebitengine.org/). 

Diseñado desde cero con foco en la precisión de sincronismo NTSC (≈59.9227 FPS), emulación acústica de calidad de estudio sin deriva ni artefactos digitales, post-procesado visual en tiempo real (CRT Scanlines, Scale2x EPX, Remaster OLED 2026) y generación automatizada de paquetes portables para **Linux**, **Windows** y **Android (APK nativo con controles táctiles HUD)**.

<p align="center">
  <img src="docs/media/features-grid.svg" alt="GoSMS Features Grid" width="100%"/>
</p>

---

## 🏛️ Arquitectura del Sistema

<p align="center">
  <img src="docs/media/architecture.svg" alt="GoSMS Architecture Diagram" width="100%"/>
</p>

### Subsistemas Emulados

| Subsistema | Implementación y Características Técnicas |
| :--- | :--- |
| **CPU Zilog Z80** | Decodificador completo de opcodes con matrices de prefijos `CB`, `ED`, `DD` y `FD`. Manejo estricto de banderas no documentadas (X/Y, F3/F5), interrupciones enmascarables modo 1 e interrupción física no enmascarable `NMI` (botón PAUSE). |
| **VDP Modo 4** | 16 KB VRAM, 32 registros CRAM con 2 paletas de 16 colores. Sincronismo exacto a 228 ciclos por scanline × 262 líneas. Sprites 8×8 y 8×16 con zoom, bit de *early clock* (desplazamiento de 8 píxeles a la izquierda), scroll dinámico horizontal/vertical e IRQs de línea de barrido programable. |
| **Memoria & Mapper** | Gestión de bancos de cartucho Sega (`$FFFD-$FFFF`), RAM de sistema de 8 KB con espejos completos y soporte de persistencia de partidas guardadas con batería SRAM (`.sav`) y snapshots en caliente (`.state`). |
| **PSG SN76489** | 3 canales de onda cuadrada independientes y generador de ruido blanco/periódico. Sincronización libre de clics a 48.000 Hz acoplada al renderizado del VDP. |
| **Audio DSP** | Procesamiento digital de señal en tiempo real: filtro paso bajo analógico IIR (~5.5 kHz), dimensionalidad 3D pseudo-estéreo (algoritmo Haas con retardo de 9 ms) y reverberación acústica de recreativa. |
| **Shaders & Vídeo** | Pipeline de post-procesado con Scale2x EPX en CPU, rejilla de scanlines CRT calibrada y shader Kage *Remaster 2026* con saturación OLED, curva de contraste gamma y bloom sutil. |

---

## 🎮 Controles y Mapeo

El emulador detecta automáticamente teclados, mandos USB/Bluetooth estándar (Xbox, PlayStation, 8BitDo) y mandos táctiles en Android:

| Función | Jugador 1 (Teclado) | Jugador 2 (Teclado) | Mando USB / Bluetooth | Android Touch HUD |
| :--- | :--- | :--- | :--- | :--- |
| **Cruceta / Movimiento** | Flechas `↑` `↓` `←` `→` | `W` `A` `S` `D` | D-Pad / Stick Izquierdo | D-Pad Virtual (Izquierda) |
| **Botón 1 (Disparo / Salto)** | `Z` o `Espacio` | `Q` | `A` (Xbox) / `✕` (PlayStation) | Botón verde `( 1 )` |
| **Botón 2 (Salto / Disparo)** | `X` | `E` | `B` (Xbox) / `◯` (PlayStation) | Botón rojo `( 2 )` |
| **Botón PAUSE (Consola)** | `P` | — | Botón `Start` | Botón `[ PAUSE ]` |
| **Cambiar Filtro de Vídeo** | `F1` o `V` | — | — | — |
| **Cambiar Filtro de Audio** | `F2` o `B` | — | — | — |
| **Guardar Partida Rápida** | `F5` | — | — | — |
| **Cargar Partida Rápida** | `F8` | — | — | — |
| **Pantalla Completa** | `F11` | — | — | — |
| **Volver al Menú / Salir** | `Esc` | — | — | — |

---

## 🚀 Compilación y Ejecución

### Requisitos Previos

- **Go 1.25** o superior (descarga automática de toolchain vía `go.mod`).
- **Docker** (únicamente necesario si deseas compilar el APK de Android en release).
- Bibliotecas nativas de desarrollo para Ebiten (OpenGL/ALSA en Linux):
  ```bash
  sudo apt-get install -y libgl1-mesa-dev xorg-dev libasound2-dev
  ```

### Comandos Rápidos

```bash
# Ejecutar emulador con la interfaz gráfica y selector interactivo
make run

# Compilar binarios de desarrollo en bin/
make build

# Ejecutar la suite de pruebas Go
make test

# Ejecutar auditoría de compatibilidad de todas las ROMs
make audit
```

### Remasterizar una ROM por objetos

El volcado conserva los tiles con sus índices de runtime y agrupa sprites contiguos en previews. Para la revisión 1 de Alex Kidd (CRC32 `aed9aac4`), además lee estáticamente las tablas RLE de mapas y metatiles de los 17 niveles; no necesita jugar ni recorrerlos con un bot. Para Alex Kidd, usa el nombre exacto de la ROM local:

```bash
make remaster ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

`make dump` también se puede invocar como `make objects`; ambos limpian primero `assets/<nombre de ROM>` para evitar mezclar índices de extracciones anteriores. Esa limpieza también elimina el workspace de IA de esa ROM: después de empezar a remasterizar, no vuelvas a ejecutar `make objects` o `make remaster` salvo que quieras regenerarlo desde cero. `FRAMES` solo controla el muestreo suplementario de sprites en ejecución; los tilesets y mapas de Alex Kidd salen de las tablas de la ROM. `make test` ejecuta `go test ./...`.

Esto genera `original/manifest.json`, PNG individuales en `original/tiles/`, previews de grupos en `original/objects/` y una imagen de índices por pantalla en `original/scenes/`. Para los mapas estáticos, cada celda del manifest conserva su `metatile_map` de 16×12 IDs y el offset del bloque en la ROM; la imagen asociada colorea los IDs para visualizar la estructura del nivel, no pretende ser una captura RGB del juego. `make unpack` copia esos mapas a `ai_workspace/scenes/` y prepara `ai_workspace/objects_contact_sheet.png`. El extractor agrupa sprites muestreados por proximidad; no adivina sus nombres. Revisa los previews y cambia el campo `label` de cada objeto a un nombre humano, por ejemplo `Alex`, `árbol` o `pez`. Para agrupar tiles de fondo, añade una entrada en `objects`, por ejemplo `{"id":"tree_001","kind":"background_object","label":"árbol","tiles":[{"index":184,"x":0,"y":0},{"index":185,"x":8,"y":0}]}`. Las posiciones son píxeles; vuelve a ejecutar `make unpack` para generar el composite de ese grupo.

Remasteriza los composites fuente de `ai_workspace/input/objects/` a escala 4×, conservando el lienzo y la cuadrícula de 32 píxeles por tile. Guarda los resultados con el mismo nombre en `ai_workspace/objects/`; así, volver a ejecutar `make unpack` no borra el trabajo de IA. Si un tile compartido necesita una versión distinta, coloca el override individual en `ai_workspace/tiles/bg_NNNN.png` o `spr_NNNN.png`. Tras cualquier cambio, ejecuta `make pack` para regenerar el spritesheet HD; al jugar, F10 activa o desactiva ese pack:

```bash
make pack ROM="roms/Alex Kidd in Miracle World.sms"
make run ROM="roms/Alex Kidd in Miracle World.sms"
```

`make hd ROM="..."` es un alias de `make pack ROM="..."`. En el juego, F10 conmuta el pack HD. El renderer sigue identificando tiles por los hashes y los índices originales del manifest; las etiquetas y previews sirven para el flujo humano/IA y no cambian la ROM.

Los objetivos `unpack`, `pack` y `hd` usan Pillow (`python3 -m pip install Pillow`).

---

## 📦 Generación de Releases Multiplataforma

El proyecto cuenta con un sistema de release totalmente automatizado en **un único comando**:

```bash
make release
```

Este comando compila de forma paralela y genera en el directorio `dist/`:

1. **🐧 Linux**: Paquete portable `dist/gosms-linux-release.zip` listo para descomprimir y jugar.
2. **🪟 Windows**: Paquete portable `dist/gosms-windows-release.zip` con ejecutable GUI optimizado (sin consola CMD emergente).
3. **📱 Android**: Archivo `dist/gosms-android/gosms.apk` compilado dentro de un contenedor Docker aislado mediante `ebitenmobile` y empaquetado nativamente con el SDK de Android (`aapt2`, `d8`, `zipalign` y `apksigner`). Incluye la ROM de GoSMS embebida y soporte táctil inmediato.

---

## 🛠️ Herramientas de Desarrollo y Diagnóstico

El repositorio incluye herramientas CLI dedicadas para análisis técnico:

- **`cmd/headless`**: Ejecuta la emulación sin interfaz gráfica para pruebas de regresión, volcados de memoria y capturas automáticas:
  ```bash
  # Ejecutar 400 frames pulsando START en el frame 120 y guardar captura PNG
  go run ./cmd/headless -frames 400 -start 120 -out captura.png ruta/juego.sms

  # Volcar audio WAV y name table para análisis de VRAM
  go run ./cmd/headless -frames 600 -wav salida.wav -nt ruta/juego.sms
  ```
- **`cmd/tracer`**: Trazador de ciclo a ciclo de instrucciones Z80 con seguimiento de registros y flags:
  ```bash
  go run ./cmd/tracer -n 5000 ruta/juego.sms
  ```

---

## 👤 Autor

Desarrollado con pasión por **Enrique González Gutiérrez**.

---

## 📄 Licencia

Este proyecto está bajo la Licencia MIT. Consulta el archivo `LICENSE` para más detalles.
