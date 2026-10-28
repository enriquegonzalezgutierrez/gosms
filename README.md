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

Diseñado desde cero con foco en la precisión de sincronismo NTSC (≈59.9227 FPS), emulación acústica de calidad de estudio sin deriva ni artefactos digitales, post-procesado visual en tiempo real (CRT Scanlines, Scale2x EPX, Remaster 2026 con sharpening, bloom y color grading) y generación automatizada de paquetes portables para **Linux**, **Windows** y **Android (APK nativo con controles táctiles HUD)**.

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
| **Shaders & Vídeo** | Pipeline multi-pase con Scale2x EPX, rejilla CRT, sharpening adaptativo, bloom multinivel y color grading. El pack HD sustituye tiles por arte remasterizado 4× u 8× y puede combinarse con el postprocesado. |

---

## 🎮 Controles y Mapeo

El emulador detecta automáticamente teclados, mandos USB/Bluetooth estándar (Xbox, PlayStation, 8BitDo) y mandos táctiles en Android:

| Función | Jugador 1 (Teclado) | Jugador 2 (Teclado) | Mando USB / Bluetooth | Android Touch HUD |
| :--- | :--- | :--- | :--- | :--- |
| **Cruceta / Movimiento** | Flechas `↑` `↓` `←` `→` | `W` `A` `S` `D` | D-Pad / Stick Izquierdo | D-Pad Virtual (Izquierda) |
| **Botón 1 (Disparo / Salto)** | `Z` o `Espacio` | `Q` | `A` (Xbox) / `✕` (PlayStation) | Botón verde `( 1 )` |
| **Botón 2 (Salto / Disparo)** | `X` | `E` | `B` (Xbox) / `◯` (PlayStation) | Botón rojo `( 2 )` |
| **Botón PAUSE / START (Consola)** | `P` o `Enter` | — | Botón `Start` | Botón `[ PAUSE ]` |
| **Cambiar Filtro de Vídeo** | `F1` o `V` | — | — | — |
| **Alternar HD / Original** | `F10` | — | — | — |
| **Luz de altura HD (rendimiento)** | `F6` | — | — | — |
| **Cambiar Filtro de Audio** | `F2` o `B` | — | — | — |
| **Guardar Partida Rápida** | `F5` | — | — | — |
| **Cargar Partida Rápida** | `F8` | — | — | — |
| **Pantalla Completa** | `F11` | — | — | — |
| **Volver al Menú / Salir** | `Esc` | — | — | — |

> En Alex Kidd, **PAUSE / START** (`P` o `Enter`, o `Start` en el mando) abre el mapa e inventario del juego. `Esc` vuelve al menú del emulador.

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

### Remasterizar una ROM

El remaster es **matemático, sin IA**: vectoriza cada objeto con campos de distancia, aplica antialiasing real y guarda un mapa de altura. El emulador usa ese mapa para iluminar la escena en tiempo real: luz direccional, especular, contraluz y sombra de los sprites. No descarga modelos y es determinista.

Para Alex Kidd, un solo comando lo hace todo (borra, extrae, vectoriza y empaqueta):

```bash
make remaster ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms"
```

Necesita Python 3 con `numpy` y `Pillow`. `make setup` crea `.venv-remaster/` con ambas.

Por defecto se genera un pack 4× (cada tile de 8×8 pasa a 32×32). Para 1440p o 4K genera el pack 8× desde el principio, porque las alturas se calculan a la misma escala que el color:

```bash
make remaster ROM="roms/Alex Kidd in Miracle World (UE) (V1.1) [!].sms" PACK_SCALE=8
```

El pack 8× se divide automáticamente en páginas de color y de altura (`pack.json` + `pages/`), con carga perezosa.

Puedes ajustar el acabado sin volver a extraer:

```bash
make cinematic ROM="..." CINE_BEVEL=2.2 CINE_LIGHT=315,48 CINE_NOISE=0.03
make pack ROM="..."
make run ROM="..."
```

`CINE_BEVEL` es el radio del bisel en píxeles de juego (1.8 por defecto): súbelo para un volumen más redondo y bájalo para un acabado más plano.

El acabado de cine (pelo castaño, piel tostada, blanco hueso y carmesí) ya es el que aplica `make remaster`. La paleta está en `tools/grades.py`: cambia un color allí, vuelve a lanzar `make cinematic` y `make pack`, y el emulador graduará también el fondo y los tiles sin remasterizar con `hd/palette.json`.

El albedo plano es el modo por defecto; para hornear la luz en el PNG usa `CINE_SHADED=1`. Para empaquetar sin mapa de altura (vuelve al relieve por gradiente del shader) usa `CINE_HEIGHTS=0`.

El pack lleva color y altura en paralelo. El filtro **REMASTER 2026** combina esa luz con bloom y tone mapping, sustituye el azul plano del cielo por un degradado con halo de sol y **F10** compara con el original. Para probar otra dirección de luz, `cmd/preview` acepta `-light "x,y"`, `-noheight` y `-nosky`. En equipos modestos, **SUAVIZADO** es el modo ligero: desactiva luz, cielo y bloom.

La extracción agrupa bloques de fondo de 8×8 tiles para vectorizar escenarios completos en vez de tiles aislados. Usa `BG_GROUP=0` para desactivarlos o `BG_PROTECT=3` para proteger más filas del HUD.

`make pack` informa de la cobertura por fuente (composites, fallback por tile, overrides manuales) y deja el detalle de los tiles compartidos en `hd/conflicts.json`.

Los detalles del algoritmo, los parámetros y las carpetas de trabajo están en la [Guía de remasterización HD](docs/GUIA_REMASTER_HD.md).

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
- **`cmd/preview`**: Renderiza un único frame a PNG con el mismo pipeline de postprocesado y el pack HD. Sirve para comparar filtros sin abrir el emulador:
  ```bash
  go run ./cmd/preview -frames 900 -filter modern -hd -out preview.png roms/juego.sms
  ```

---

## 👤 Autor

Desarrollado con pasión por **Enrique González Gutiérrez**.

---

## 📄 Licencia

Este proyecto está bajo la Licencia MIT. Consulta el archivo `LICENSE` para más detalles.
