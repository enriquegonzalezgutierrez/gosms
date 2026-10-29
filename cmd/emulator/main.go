// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command emulator arranca una Sega Master System con interfaz moderna,
// selector de ROMs y filtros de post-procesado de vídeo y audio en tiempo real.
// Desarrollado por Enrique González Gutiérrez.
package main

import (
	_ "embed"

	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"gosms/pkg/machine"
	"gosms/pkg/vdp"
	"gosms/pkg/video"
)

const (
	authorName      = "Enrique González Gutiérrez"
	sampleRate      = 48000
	samplesPerFrame = 801 // 48000 Hz / 59.9227 FPS (NTSC)
	menuW           = 800
	menuH           = 520
)

// ---------- Filtros de Vídeo y Audio ----------

type VideoFilter int

const (
	VideoFilterPixelPerfect VideoFilter = iota
	VideoFilterScanlines
	VideoFilterEPX
	VideoFilterSmooth
	VideoFilterMonochrome
	VideoFilterRemaster2026
)

var videoFilterNames = []string{
	"PÍXEL PERFECT (NÍTIDO)",
	"CRT SCANLINES (TV DE TUBO)",
	"SCALE2X EPX (HD SUAVE)",
	"SUAVIZADO BILINEAL",
	"FÓSFORO VERDE MONOCROMO",
	"REMASTER 2026 (NÍTIDO + BLOOM)",
}

type AudioFilter int

const (
	AudioFilterRaw AudioFilter = iota
	AudioFilterWarm
	AudioFilterSpatial
	AudioFilterReverb
)

var audioFilterNames = []string{
	"AUDIO PURO (SN76489 DIRECTO)",
	"CALIDEZ ANALÓGICA CRT (PASO BAJO)",
	"PSEUDO-ESTÉREO 3D (HAAS)",
	"SALÓN RECREATIVO (REVERB)",
}

// ---------- DSP de Audio ----------

type AudioDSP struct {
	// Filtro Paso Bajo IIR (LPF)
	lpfPrevL float64
	lpfPrevR float64

	// Retardo espacial Haas (Pseudo-Estéreo)
	haasRing [512]float64
	haasIdx  int

	// Reverb con delay line y feedback
	reverbRing [2400]float64
	reverbIdx  int
}

func (dsp *AudioDSP) Process(samples []int16, mode AudioFilter) {
	if mode == AudioFilterRaw {
		return
	}

	for i := 0; i < len(samples); i += 2 {
		inL := float64(samples[i])
		inR := float64(samples[i+1])

		var outL, outR float64

		switch mode {
		case AudioFilterWarm:
			// Filtro paso bajo IIR (~5.5 kHz a 48 kHz). Suaviza los armónicos ásperos de la onda cuadrada
			const alpha = 0.45
			dsp.lpfPrevL += alpha * (inL - dsp.lpfPrevL)
			dsp.lpfPrevR += alpha * (inR - dsp.lpfPrevR)
			outL = dsp.lpfPrevL
			outR = dsp.lpfPrevR

		case AudioFilterSpatial:
			// Canal izquierdo directo; canal derecho con micro-retardo de ~9 ms para dimensionalidad 3D
			delayedR := dsp.haasRing[dsp.haasIdx]
			dsp.haasRing[dsp.haasIdx] = inR
			dsp.haasIdx = (dsp.haasIdx + 1) % len(dsp.haasRing)

			outL = inL*0.85 + inR*0.15
			outR = delayedR*0.85 + inL*0.15

		case AudioFilterReverb:
			// Efecto de eco y reverberación de habitación / arcade
			echo := dsp.reverbRing[dsp.reverbIdx]
			dsp.reverbRing[dsp.reverbIdx] = inL*0.6 + echo*0.38
			dsp.reverbIdx = (dsp.reverbIdx + 1) % len(dsp.reverbRing)

			outL = inL + echo*0.45
			outR = inR + echo*0.45
		}

		// Clamping para evitar distorsión digital
		if outL > 32767 {
			outL = 32767
		} else if outL < -32768 {
			outL = -32768
		}
		if outR > 32767 {
			outR = 32767
		} else if outR < -32768 {
			outR = -32768
		}

		samples[i] = int16(outL)
		samples[i+1] = int16(outR)
	}
}

// ---------- Buffer de Audio ----------

type audioBuffer struct {
	mu   sync.Mutex
	cond *sync.Cond
	buf  []byte
}

func newAudioBuffer() *audioBuffer {
	b := &audioBuffer{}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (a *audioBuffer) Write(p []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	const maxBuffer = (sampleRate * 4 * 100) / 1000 // 100 ms
	if len(a.buf) > maxBuffer {
		a.buf = a.buf[len(a.buf)-maxBuffer:]
	}

	a.buf = append(a.buf, p...)
	a.cond.Signal()
}

func (a *audioBuffer) Read(p []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for len(a.buf) == 0 {
		a.cond.Wait()
	}

	n := copy(p, a.buf)
	a.buf = a.buf[n:]
	return n, nil
}

func (a *audioBuffer) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.buf = nil
}

// ---------- Entrada ----------

type keyBit struct {
	key ebiten.Key
	bit byte
}

var pad1Keys = []keyBit{
	{ebiten.KeyArrowUp, 0x01},
	{ebiten.KeyArrowDown, 0x02},
	{ebiten.KeyArrowLeft, 0x04},
	{ebiten.KeyArrowRight, 0x08},
	{ebiten.KeyZ, 0x10}, // Botón 1
	{ebiten.KeyX, 0x20}, // Botón 2
}

var pad2Keys = []keyBit{
	{ebiten.KeyW, 0x01},
	{ebiten.KeyS, 0x02},
	{ebiten.KeyA, 0x04},
	{ebiten.KeyD, 0x08},
	{ebiten.KeyQ, 0x10},
	{ebiten.KeyE, 0x20},
}

func readPad(keys []keyBit, padIndex int) byte {
	v := byte(0xFF)
	for _, kb := range keys {
		if ebiten.IsKeyPressed(kb.key) {
			v &^= kb.bit
		}
	}

	// Mapeo automático de mandos USB / Bluetooth
	var ids []ebiten.GamepadID
	ids = ebiten.AppendGamepadIDs(ids)
	if padIndex < len(ids) {
		id := ids[padIndex]
		// D-Pad y Joystick analógico
		if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftTop) ||
			ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical) < -0.4 {
			v &^= 0x01 // Arriba
		}
		if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftBottom) ||
			ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical) > 0.4 {
			v &^= 0x02 // Abajo
		}
		if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftLeft) ||
			ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal) < -0.4 {
			v &^= 0x04 // Izquierda
		}
		if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftRight) ||
			ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal) > 0.4 {
			v &^= 0x08 // Derecha
		}
		// Botón 1 (A en Xbox, Cruz en PlayStation)
		if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightBottom) ||
			ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightLeft) {
			v &^= 0x10
		}
		// Botón 2 (B en Xbox, Círculo en PlayStation)
		if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightRight) ||
			ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightTop) {
			v &^= 0x20
		}
	}
	return v
}

func isGamepadButtonJustPressed(b ebiten.StandardGamepadButton) bool {
	for _, id := range ebiten.AppendGamepadIDs(nil) {
		if inpututil.IsStandardGamepadButtonJustPressed(id, b) {
			return true
		}
	}
	return false
}

func gamepadAxis(axis ebiten.StandardGamepadAxis) int {
	for _, id := range ebiten.AppendGamepadIDs(nil) {
		value := ebiten.StandardGamepadAxisValue(id, axis)
		if value > 0.6 {
			return 1
		}
		if value < -0.6 {
			return -1
		}
	}
	return 0
}

// ---------- Metadatos de ROM y Miniaturas Inteligentes ----------

type RomItem struct {
	Path      string
	Title     string
	Thumbnail *ebiten.Image
}

func cleanTitle(filename string) string {
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	words := strings.Split(name, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func evalFrameScore(frame []byte) int {
	colorCounts := make(map[uint16]int)
	totalSamples := 0

	for i := 0; i < len(frame); i += 16 {
		r, g, b := frame[i]>>3, frame[i+1]>>3, frame[i+2]>>3
		c := uint16(r)<<10 | uint16(g)<<5 | uint16(b)
		colorCounts[c]++
		totalSamples++
	}

	if totalSamples == 0 || len(colorCounts) <= 1 {
		return 0
	}

	maxCount := 0
	for _, count := range colorCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	// Descartar solo si mas del 97% es un unico color plano (pantalla vacia)
	if float64(maxCount)/float64(totalSamples) > 0.97 {
		return 0
	}

	nonBgPixels := totalSamples - maxCount
	if nonBgPixels < 40 {
		return 0
	}

	return (len(colorCounts) * 100) + nonBgPixels
}

func generateSmartThumbnail(romPath string) (*ebiten.Image, error) {
	thumbDir := ".thumbnails"
	_ = os.MkdirAll(thumbDir, 0755)

	baseName := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	thumbPath := filepath.Join(thumbDir, baseName+".png")

	if f, err := os.Open(thumbPath); err == nil {
		defer f.Close()
		img, err := png.Decode(f)
		if err == nil {
			return ebiten.NewImageFromImage(img), nil
		}
	}

	romData, err := os.ReadFile(romPath)
	if err != nil {
		return nil, err
	}

	m := machine.New(romData)
	var bestFrame []byte
	bestScore := -1

	for i := 0; i < 450; i++ {
		// Pulsaciones de boton 1 y 2 para saltar intros de SEGA y llegar al titulo
		if i%45 >= 10 && i%45 <= 20 {
			m.CPU.Pad1 = 0xCF // Boton 1 y Boton 2 pulsados
		} else {
			m.CPU.Pad1 = 0xFF
		}
		m.RunFrame()

		if i >= 40 {
			score := evalFrameScore(m.VDP.Frame[:])
			if score > bestScore {
				bestScore = score
				if bestFrame == nil {
					bestFrame = make([]byte, len(m.VDP.Frame))
				}
				copy(bestFrame, m.VDP.Frame[:])
			}
		}
	}

	if bestFrame == nil {
		bestFrame = m.VDP.Frame[:]
	}

	rgba := image.NewRGBA(image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH))
	copy(rgba.Pix, bestFrame)

	if outF, err := os.Create(thumbPath); err == nil {
		_ = png.Encode(outF, rgba)
		outF.Close()
	}

	return ebiten.NewImageFromImage(rgba), nil
}

// ---------- Estados de la Aplicación ----------

type AppState int

const (
	StateLoading AppState = iota
	StateMenu
	StatePlaying
)

type game struct {
	state      AppState
	romFiles   []string
	roms       []RomItem
	loadingIdx int
	loadingMsg string
	loadChan   chan RomItem
	loadDone   chan bool
	selected   int
	curTitle   string
	curRomPath string
	m          *machine.Machine
	img        *ebiten.Image
	fullscreen bool
	crop       bool
	hdMode     bool
	dumper     *LiveDumper
	audioBuf   *audioBuffer
	dsp        AudioDSP
	monoBuf    []byte
	stereoBuf  []byte
	samplesBuf []int16
	frames     int

	// Medición de rendimiento con -bench.
	benchFrames  int
	benchWarmup  int
	benchPlay    int
	benchDone    int
	benchStart   time.Time
	benchPresses []benchPress
	benchDefault bool

	// Pipeline de post-procesado compartido con Android.
	pipeline *video.Pipeline
	hdTarget *ebiten.Image

	// Filtros de post-procesado
	videoFilter VideoFilter
	audioFilter AudioFilter

	// Notificación en pantalla (OSD)
	osdText  string
	osdTimer int

	// Mandos conectados y su modelo (hotplug).
	padIDs   []ebiten.GamepadID
	padNames map[ebiten.GamepadID]string

	// Ayuda/tutorial de controles.
	showHelp  bool
	menuAxisX int
	menuAxisY int

	// Textura temporal del upscaler EPX.
	epxImg *ebiten.Image
	epxBuf []byte
}

type benchPress struct {
	start int
	pad   byte
	len   int
}

func parseBenchScript(s string) []benchPress {
	var out []benchPress
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, ":")
		if len(fields) < 2 {
			continue
		}
		var start int
		var pad uint
		length := 8
		fmt.Sscanf(fields[0], "%d", &start)
		fmt.Sscanf(fields[1], "%x", &pad)
		if len(fields) > 2 {
			fmt.Sscanf(fields[2], "%d", &length)
		}
		out = append(out, benchPress{start: start, pad: byte(pad), len: length})
	}
	return out
}

func defaultBenchScript() []benchPress {
	// Un bot genérico: pulsa START varias veces para saltar títulos/menús y
	// después mantiene derecha. Los saltos se generan en benchPad para no
	// reservar millones de entradas.
	return []benchPress{
		{start: 300, pad: 0xEF, len: 8},
		{start: 600, pad: 0xEF, len: 8},
		{start: 900, pad: 0xEF, len: 8},
		{start: 1200, pad: 0xEF, len: 8},
		{start: 1500, pad: 0xF7, len: 1 << 30}, // derecha indefinida
	}
}

func (g *game) benchPad(frame int) byte {
	pad := byte(0xFF)
	for _, p := range g.benchPresses {
		if frame >= p.start && frame < p.start+p.len {
			pad &= p.pad
		}
	}
	if g.benchDefault && frame >= 1600 && (frame-1600)%90 < 8 {
		pad &= 0xD7 // derecha + botón 2
	}
	return pad
}

func (g *game) loadGame(path string) error {
	rom, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	g.curRomPath = path
	g.m = machine.New(rom)
	machine.InitHD(path)
	g.hdMode = false
	g.dumper = newLiveDumper(path)
	g.m.CPU.PSG.SetSampleRate(sampleRate)
	g.audioBuf.Reset()

	// Cargar archivo .sav de batería de guardado si existe
	sramPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".sav"
	g.m.CPU.LoadSRAM(sramPath)
	return nil
}

func (g *game) showOSD(msg string) {
	g.osdText = msg
	g.osdTimer = 110 // ~1.8 segundos a 60 FPS
}

func (g *game) closeHelp() {
	g.showHelp = false
	cfg := loadConfig()
	if !cfg.TutorialSeen {
		cfg.TutorialSeen = true
		saveConfig(cfg)
	}
}

func shortPadName(name string) string {
	runes := []rune(name)
	if len(runes) > 40 {
		return string(runes[:37]) + "..."
	}
	return name
}

// detectGamepads follows Ebiten's hotplug list and announces new or removed
// controllers with their model name. It runs in the menu and in game.
func (g *game) detectGamepads() {
	if g.padNames == nil {
		g.padNames = make(map[ebiten.GamepadID]string)
	}
	ids := ebiten.AppendGamepadIDs(nil)
	seen := make(map[ebiten.GamepadID]bool, len(ids))
	for index, id := range ids {
		seen[id] = true
		if _, known := g.padNames[id]; known {
			continue
		}
		name := strings.TrimSpace(ebiten.GamepadName(id))
		if name == "" {
			name = "Mando USB/Bluetooth"
		}
		g.padNames[id] = name
		g.showOSD(fmt.Sprintf("🎮 MANDO %d DETECTADO: %s", index+1, shortPadName(name)))
	}
	for id, name := range g.padNames {
		if seen[id] {
			continue
		}
		delete(g.padNames, id)
		g.showOSD("🔌 MANDO DESCONECTADO: " + shortPadName(name))
	}
	g.padIDs = ids
}

func (g *game) gamepadStatus() string {
	if len(g.padIDs) == 0 {
		return "MANDO: ninguno  ·  conecta uno USB/Bluetooth"
	}
	name := g.padNames[g.padIDs[0]]
	if name == "" {
		name = "Mando USB/Bluetooth"
	}
	if len(g.padIDs) == 1 {
		return "MANDO: " + shortPadName(name)
	}
	return fmt.Sprintf("MANDOS: %d  ·  %s", len(g.padIDs), shortPadName(name))
}

func (g *game) Update() error {
	g.frames++
	if g.benchFrames > 0 && g.state == StatePlaying {
		g.benchPlay++
		if g.benchPlay > g.benchWarmup {
			if g.benchDone == 0 {
				g.benchStart = time.Now()
			}
			if g.benchDone >= g.benchFrames {
				elapsed := time.Since(g.benchStart)
				fps := float64(g.benchDone) / elapsed.Seconds()
				w, h := ebiten.WindowSize()
				fmt.Printf("bench frames=%d warmup=%d elapsed=%.3fs ms_per_frame=%.3f fps=%.1f hd=%v filter=%s size=%dx%d\n",
					g.benchDone, g.benchWarmup, elapsed.Seconds(), elapsed.Seconds()*1000/float64(g.benchDone), fps,
					g.hdMode, videoFilterNames[g.videoFilter], w, h)
				return ebiten.Termination
			}
			g.benchDone++
		}
	}
	if g.osdTimer > 0 {
		g.osdTimer--
	}
	g.detectGamepads()

	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		g.fullscreen = !g.fullscreen
		ebiten.SetFullscreen(g.fullscreen)
	}

	// Teclas rápidas globales para cambiar filtros
	// F8: Pantallazo limpio para IA
	if inpututil.IsKeyJustPressed(ebiten.KeyF7) {
		if g.dumper != nil && g.img != nil {
			if g.m != nil && g.m.VDP != nil {
				g.img.WritePixels(g.m.VDP.Frame[:])
			}
			shot := g.dumper.SaveScreenshot(g.img)
			if shot != "" {
				g.showOSD("📸 PANTALLAZO IA: " + shot)
			}
		}
	}

	// F9: Iniciar / Detener grabación de Dump en vivo
	if inpututil.IsKeyJustPressed(ebiten.KeyF9) {
		if g.dumper != nil {
			g.dumper.Active = !g.dumper.Active
			if g.dumper.Active {
				g.showOSD("🔴 GRABANDO DUMP EN VIVO (F9 para guardar)")
			} else {
				saved := g.dumper.Save()
				if saved > 0 {
					g.showOSD(fmt.Sprintf("💾 DUMP GUARDADO: +%d NUEVOS tiles! (Total: %d)", saved, len(g.dumper.Manifest)))
				} else {
					g.showOSD(fmt.Sprintf("💾 DUMP OK: Zona ya volcada al 100%% (%d tiles)", len(g.dumper.Manifest)))
				}
			}
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF10) {
		if machine.HD != nil && machine.HD.Active {
			g.hdMode = !g.hdMode
			if g.hdMode {
				g.showOSD("MODO HD: ACTIVADO")
			} else {
				g.showOSD("MODO HD: DESACTIVADO")
			}
		} else {
			g.showOSD("PACK HD NO DISPONIBLE PARA ESTE JUEGO")
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF1) || inpututil.IsKeyJustPressed(ebiten.KeyV) {
		g.videoFilter = (g.videoFilter + 1) % 6
		g.showOSD(fmt.Sprintf("VÍDEO: %s", videoFilterNames[g.videoFilter]))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) || inpututil.IsKeyJustPressed(ebiten.KeyB) {
		g.audioFilter = (g.audioFilter + 1) % 4
		g.showOSD(fmt.Sprintf("AUDIO: %s", audioFilterNames[g.audioFilter]))
	}

	if g.state != StateLoading {
		if inpututil.IsKeyJustPressed(ebiten.KeyF12) || inpututil.IsKeyJustPressed(ebiten.KeyH) {
			if g.showHelp {
				g.closeHelp()
			} else {
				g.showHelp = true
			}
		}
		if g.showHelp {
			if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonRightBottom) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonRightRight) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonCenterRight) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonCenterLeft) {
				g.closeHelp()
			}
			return nil
		}
	}

	switch g.state {
	case StateLoading:
		select {
		case item := <-g.loadChan:
			g.roms = append(g.roms, item)
			g.loadingIdx++
			if g.loadingIdx < len(g.romFiles) {
				g.loadingMsg = cleanTitle(g.romFiles[g.loadingIdx])
			}
		case <-g.loadDone:
			g.state = StateMenu
			ebiten.SetWindowTitle("gosms - Sega Master System Launcher")
		default:
		}

	case StateMenu:
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
			isGamepadButtonJustPressed(ebiten.StandardGamepadButtonRightRight) ||
			isGamepadButtonJustPressed(ebiten.StandardGamepadButtonCenterLeft) {
			return ebiten.Termination
		}
		// Tecla F3 para cambiar/recargar carpeta de roms
		if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
			cfg := loadConfig()
			// Alternar o buscar en directorio actual por defecto
			if cfg.RomsDir == "roms" {
				cfg.RomsDir = "."
			} else {
				cfg.RomsDir = "roms"
			}
			saveConfig(cfg)
			g.selected = 0
		}

		if len(g.roms) > 0 {
			// Tecla R: Regenerar carátula del juego seleccionado
			if inpututil.IsKeyJustPressed(ebiten.KeyR) {
				g.showOSD("REGENERANDO CARÁTULA...")
				cur := g.roms[g.selected]
				baseName := strings.TrimSuffix(filepath.Base(cur.Path), filepath.Ext(cur.Path))
				os.Remove(filepath.Join(".thumbnails", baseName+".png"))
				if thumb, err := generateSmartThumbnail(cur.Path); err == nil {
					g.roms[g.selected].Thumbnail = thumb
				}
			}
			axisX := gamepadAxis(ebiten.StandardGamepadAxisLeftStickHorizontal)
			axisY := gamepadAxis(ebiten.StandardGamepadAxisLeftStickVertical)
			dx, dy := 0, 0
			if inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonLeftRight) {
				dx++
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonLeftLeft) {
				dx--
			}
			if dx == 0 && axisX != 0 && axisX != g.menuAxisX {
				dx = axisX
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyDown) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonLeftBottom) {
				dy++
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyUp) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonLeftTop) {
				dy--
			}
			if dy == 0 && axisY != 0 && axisY != g.menuAxisY {
				dy = axisY
			}
			g.menuAxisX, g.menuAxisY = axisX, axisY
			if dx > 0 || dy > 0 {
				g.selected = (g.selected + 1) % len(g.roms)
			}
			if dx < 0 || dy < 0 {
				g.selected = (g.selected - 1 + len(g.roms)) % len(g.roms)
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonRightBottom) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonCenterRight) {
				if err := g.loadGame(g.roms[g.selected].Path); err != nil {
					log.Printf("Error cargando juego: %v", err)
				} else {
					g.state = StatePlaying
					g.curTitle = g.roms[g.selected].Title
				}
			}
		}

	case StatePlaying:
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			if g.m != nil {
				g.m.CPU.SaveSRAM()
			}
			g.state = StateMenu
			g.m = nil
			g.audioBuf.Reset()
			ebiten.SetWindowTitle("gosms - Sega Master System Launcher")
			return nil
		}

		if g.frames%30 == 0 {
			ebiten.SetWindowTitle(fmt.Sprintf("gosms - %s   %.0f FPS  •  %.1f TPS",
				g.curTitle, ebiten.ActualFPS(), ebiten.ActualTPS()))
		}

		// Botón PAUSE físico de la consola (P/Enter o Start del mando)
		if inpututil.IsKeyJustPressed(ebiten.KeyP) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
			isGamepadButtonJustPressed(ebiten.StandardGamepadButtonCenterRight) {
			g.m.CPU.NMI = true
			g.showOSD("PAUSE / START (NMI)")
		}

		// Save State: Guardar partida rápida con F5
		if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
			statePath := strings.TrimSuffix(g.curRomPath, filepath.Ext(g.curRomPath)) + ".state"
			if err := g.m.SaveState(statePath); err != nil {
				g.showOSD("ERROR GUARDANDO ESTADO")
			} else {
				g.showOSD("PARTIDA GUARDADA (F5)")
			}
		}

		// Save State: Cargar partida rápida con F8
		if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
			statePath := strings.TrimSuffix(g.curRomPath, filepath.Ext(g.curRomPath)) + ".state"
			if err := g.m.LoadState(statePath); err != nil {
				g.showOSD("NO HAY PARTIDA GUARDADA")
			} else {
				g.showOSD("PARTIDA CARGADA (F8)")
			}
		}

		// Auto-guardado periódico de SRAM (batería de guardado) cada 5 segundos
		if g.frames%300 == 0 {
			g.m.CPU.SaveSRAM()
		}

		g.m.CPU.Pad1 = readPad(pad1Keys, 0)
		g.m.CPU.Pad2 = readPad(pad2Keys, 1)
		if g.benchFrames > 0 {
			g.m.CPU.Pad1 = g.benchPad(g.benchPlay)
		}
		g.m.RunFrame()

		// Capturar frame recién procesado si el dumper está activo
		if g.dumper != nil && g.dumper.Active && g.m != nil && g.m.VDP != nil {
			g.dumper.CaptureFrame(g.m.VDP)
		}

		// 1. Generar audio mono base del PSG
		monoBytes := samplesPerFrame * 2
		if len(g.monoBuf) < monoBytes {
			g.monoBuf = make([]byte, monoBytes)
		}
		g.m.PSG.Render(g.monoBuf[:monoBytes])

		// 2. Convertir a int16 estéreo
		if len(g.samplesBuf) < samplesPerFrame*2 {
			g.samplesBuf = make([]int16, samplesPerFrame*2)
		}
		for i := 0; i < samplesPerFrame; i++ {
			val := int16(uint16(g.monoBuf[i*2]) | uint16(g.monoBuf[i*2+1])<<8)
			g.samplesBuf[i*2] = val
			g.samplesBuf[i*2+1] = val
		}

		// 3. Aplicar filtro de audio DSP seleccionado
		g.dsp.Process(g.samplesBuf[:samplesPerFrame*2], g.audioFilter)

		// 4. Encolar PCM en el buffer de reproducción
		stereoBytes := samplesPerFrame * 4
		if len(g.stereoBuf) < stereoBytes {
			g.stereoBuf = make([]byte, stereoBytes)
		}
		for i := 0; i < samplesPerFrame*2; i++ {
			s := g.samplesBuf[i]
			g.stereoBuf[i*2] = byte(s)
			g.stereoBuf[i*2+1] = byte(s >> 8)
		}
		g.audioBuf.Write(g.stereoBuf[:stereoBytes])
	}

	return nil
}

func (g *game) drawLoading(screen *ebiten.Image) {
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	screen.Fill(color.RGBA{11, 15, 21, 255})

	vector.DrawFilledRect(screen, 0, 0, float32(sw), 54, color.RGBA{9, 12, 18, 255}, true)
	vector.DrawFilledRect(screen, 0, 54, float32(sw), 2, color.RGBA{0, 140, 255, 255}, true)
	vector.DrawFilledRect(screen, 24, 13, 68, 28, color.RGBA{0, 85, 204, 255}, true)
	ebitenutil.DebugPrintAt(screen, "SEGA", 42, 19)
	ebitenutil.DebugPrintAt(screen, "GOSMS  •  INICIALIZANDO BIBLIOTECA", 106, 19)

	authorBadge := fmt.Sprintf("DEV: %s", strings.ToUpper(authorName))
	authorX := sw - len(authorBadge)*6 - 36
	vector.DrawFilledRect(screen, float32(authorX-8), 13, float32(len(authorBadge)*6+24), 28, color.RGBA{22, 27, 36, 255}, true)
	ebitenutil.DebugPrintAt(screen, authorBadge, authorX+4, 19)

	centerX := float32(sw) / 2
	centerY := float32(sh)/2 - 30
	radius := float32(36)

	dots := 12
	baseAngle := float64(g.frames) * 0.12
	for i := 0; i < dots; i++ {
		angle := baseAngle + float64(i)*(2*math.Pi/float64(dots))
		px := centerX + radius*float32(math.Cos(angle))
		py := centerY + radius*float32(math.Sin(angle))

		alpha := uint8(40 + (i * 215 / dots))
		dotSize := float32(2.5 + float64(i)*0.2)
		vector.DrawFilledCircle(screen, px, py, dotSize, color.RGBA{0, 160, 255, alpha}, true)
	}

	statusTitle := "GENERANDO CARATULAS INTELIGENTES..."
	ebitenutil.DebugPrintAt(screen, statusTitle, sw/2-len(statusTitle)*3, int(centerY)+60)

	progressText := fmt.Sprintf("Analizando: %s [ %d / %d ]", g.loadingMsg, g.loadingIdx, len(g.romFiles))
	ebitenutil.DebugPrintAt(screen, progressText, sw/2-len(progressText)*3, int(centerY)+84)
}

func (g *game) drawCard(screen *ebiten.Image, item RomItem, cx, cy, w, h float32, isCenter bool) {
	x := cx - w/2
	y := cy - h/2

	if isCenter {
		pulse := float32((math.Sin(float64(g.frames)*0.08) + 1.0) / 2.0)
		r := uint8(0 + pulse*60)
		gr := uint8(140 + pulse*80)
		bl := uint8(255)

		vector.DrawFilledRect(screen, x-5, y-5, w+10, h+10, color.RGBA{r, gr, bl, 255}, true)
		vector.DrawFilledRect(screen, x-2, y-2, w+4, h+4, color.RGBA{230, 245, 255, 220}, true)
	} else {
		vector.DrawFilledRect(screen, x-2, y-2, w+4, h+4, color.RGBA{45, 55, 72, 255}, true)
	}

	vector.DrawFilledRect(screen, x, y, w, h, color.RGBA{20, 24, 33, 255}, true)

	if item.Thumbnail != nil {
		scaleX := float64(w) / float64(vdp.ScreenW)
		scaleY := float64(h) / float64(vdp.ScreenH)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scaleX, scaleY)
		op.GeoM.Translate(float64(x), float64(y))
		if !isCenter {
			op.ColorScale.Scale(0.35, 0.35, 0.45, 1.0)
		}
		op.Filter = ebiten.FilterLinear
		screen.DrawImage(item.Thumbnail, op)
	}
}

func (g *game) drawMenu(screen *ebiten.Image) {
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()

	screen.Fill(color.RGBA{11, 15, 21, 255})
	vector.DrawFilledRect(screen, 0, 0, float32(sw), float32(sh)/2, color.RGBA{16, 22, 34, 255}, false)

	// Cabecera superior
	vector.DrawFilledRect(screen, 0, 0, float32(sw), 54, color.RGBA{9, 12, 18, 255}, true)
	vector.DrawFilledRect(screen, 0, 54, float32(sw), 2, color.RGBA{0, 140, 255, 255}, true)

	vector.DrawFilledRect(screen, 24, 13, 68, 28, color.RGBA{0, 85, 204, 255}, true)
	ebitenutil.DebugPrintAt(screen, "SEGA", 42, 19)
	ebitenutil.DebugPrintAt(screen, "GOSMS  •  MASTER SYSTEM", 106, 19)

	ebitenutil.DebugPrintAt(screen, g.gamepadStatus(), 24, 62)
	filterText := fmt.Sprintf("VÍDEO: %s   ·   AUDIO: %s",
		videoFilterNames[g.videoFilter], audioFilterNames[g.audioFilter])
	ebitenutil.DebugPrintAt(screen, filterText, 24, 80)

	authorBadge := fmt.Sprintf("DEV: %s", strings.ToUpper(authorName))
	authorX := sw - len(authorBadge)*6 - 36
	vector.DrawFilledRect(screen, float32(authorX-8), 13, float32(len(authorBadge)*6+24), 28, color.RGBA{22, 27, 36, 255}, true)
	vector.DrawFilledRect(screen, float32(authorX-8), 13, float32(len(authorBadge)*6+24), 28, color.RGBA{0, 128, 255, 120}, false)
	ebitenutil.DebugPrintAt(screen, authorBadge, authorX+4, 19)

	if len(g.roms) == 0 {
		ebitenutil.DebugPrintAt(screen, "No se encontraron juegos en la carpeta ./roms", sw/2-140, sh/2)
		return
	}

	centerY := float32(sh) / 2
	mainW := float32(360)
	mainH := float32(270)
	sideW := float32(240)
	sideH := float32(180)

	prevIdx := (g.selected - 1 + len(g.roms)) % len(g.roms)
	nextIdx := (g.selected + 1) % len(g.roms)

	if len(g.roms) > 1 {
		g.drawCard(screen, g.roms[prevIdx], float32(sw)/2-280, centerY-15, sideW, sideH, false)
	}
	if len(g.roms) > 2 {
		g.drawCard(screen, g.roms[nextIdx], float32(sw)/2+280, centerY-15, sideW, sideH, false)
	}

	g.drawCard(screen, g.roms[g.selected], float32(sw)/2, centerY-15, mainW, mainH, true)

	cur := g.roms[g.selected]
	titleText := fmt.Sprintf("<   %s   >", strings.ToUpper(cur.Title))
	ebitenutil.DebugPrintAt(screen, titleText, sw/2-len(titleText)*3, int(centerY+mainH/2)+14)

	pageText := fmt.Sprintf("[  %d / %d  ]", g.selected+1, len(g.roms))
	ebitenutil.DebugPrintAt(screen, pageText, sw/2-len(pageText)*3, int(centerY+mainH/2)+32)

	// Barra inferior
	barH := float32(64)
	vector.DrawFilledRect(screen, 0, float32(sh)-barH, float32(sw), barH, color.RGBA{9, 12, 18, 255}, true)
	vector.DrawFilledRect(screen, 0, float32(sh)-barH, float32(sw), 1, color.RGBA{0, 140, 255, 180}, true)

	hint1 := "[ ENTER / A ] Jugar    [ D-PAD / STICK ] Elegir    [ F12 ] Controles    [ F11 ] Pantalla"
	ebitenutil.DebugPrintAt(screen, hint1, sw/2-len(hint1)*3, sh-50)

	hint2 := "[ F3 ] Carpeta ROMs    [ F1 ] Vídeo    [ F2 ] Audio    [ ESC ] Salir"
	ebitenutil.DebugPrintAt(screen, hint2, sw/2-len(hint2)*3, sh-32)

	credit := fmt.Sprintf("gosms  ©  Creado por %s", authorName)
	ebitenutil.DebugPrintAt(screen, credit, sw/2-len(credit)*3, sh-16)
}

func (g *game) drawHelp(screen *ebiten.Image) {
	sw := float64(screen.Bounds().Dx())
	sh := float64(screen.Bounds().Dy())

	vector.DrawFilledRect(screen, 0, 0, float32(sw), float32(sh), color.RGBA{0, 0, 0, 210}, true)
	w := math.Min(sw-60, 700)
	h := math.Min(sh-60, 440)
	x := (sw - w) / 2
	y := (sh - h) / 2

	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{16, 22, 34, 250}, true)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 2, color.RGBA{0, 170, 255, 240}, true)

	title := "CONTROLES Y ATAJOS"
	ebitenutil.DebugPrintAt(screen, title, int(x+w/2)-len(title)*3, int(y+16))
	subtitle := "Tutorial de primer arranque · F12 / H para volver a abrirlo"
	ebitenutil.DebugPrintAt(screen, subtitle, int(x+w/2)-len(subtitle)*3, int(y+34))

	drawLine := func(cx, cy float64, key, desc string) {
		text := fmt.Sprintf("%-18s %s", key, desc)
		ebitenutil.DebugPrintAt(screen, text, int(cx), int(cy))
	}

	col1 := x + 28
	col2 := x + w/2 + 8
	y0 := y + 66
	ebitenutil.DebugPrintAt(screen, "JUEGO", int(col1), int(y0))
	ebitenutil.DebugPrintAt(screen, "MENÚ", int(col2), int(y0))
	left := [][2]string{
		{"Flechas", "Mover"},
		{"Z / Espacio", "Botón 1"},
		{"X", "Botón 2"},
		{"P / Enter", "PAUSE / START"},
		{"F5 / F8", "Guardar / cargar"},
		{"F1 / F2", "Vídeo / audio"},
		{"F10", "HD / original"},
		{"F11", "Pantalla completa"},
		{"Esc", "Volver al menú"},
	}
	right := [][2]string{
		{"Enter / A / Start", "Jugar"},
		{"D-pad / stick", "Elegir juego"},
		{"F3", "Carpeta de ROMs"},
		{"R", "Regenerar carátula"},
		{"F12 / H", "Abrir / cerrar ayuda"},
		{"B / Back / Esc", "Salir"},
		{"F9 / F7", "Herramientas de desarrollo"},
	}
	for i, line := range left {
		drawLine(col1, y0+20+float64(i)*18, line[0], line[1])
	}
	for i, line := range right {
		drawLine(col2, y0+20+float64(i)*18, line[0], line[1])
	}

	pad := g.gamepadStatus()
	ebitenutil.DebugPrintAt(screen, pad, int(x+28), int(y+h-44))
	closeText := "F12 / A / Start / Enter / Esc: cerrar"
	ebitenutil.DebugPrintAt(screen, closeText, int(x+w/2)-len(closeText)*3, int(y+h-24))
}

func (g *game) Draw(screen *ebiten.Image) {
	switch g.state {
	case StateLoading:
		g.drawLoading(screen)
		return

	case StateMenu:
		g.drawMenu(screen)
		if g.showHelp {
			g.drawHelp(screen)
		}
		return
	}

	if g.m == nil || g.m.VDP == nil {
		return
	}

	// 1. Cargar píxeles del VDP (el HD compone desde VRAM, sin este upload)
	if !(g.hdMode && machine.HD != nil && machine.HD.Active) {
		g.img.WritePixels(g.m.VDP.Frame[:])
	}

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	source, srcRect := g.frameSource()
	if source == nil || srcRect.Empty() {
		return
	}

	srcW, srcH := srcRect.Dx(), srcRect.Dy()
	scale := math.Min(float64(sw)/float64(srcW), float64(sh)/float64(srcH))
	dstW := int(math.Round(float64(srcW) * scale))
	dstH := int(math.Round(float64(srcH) * scale))
	destX := (sw - dstW) / 2
	destY := (sh - dstH) / 2
	dstRect := image.Rect(destX, destY, destX+dstW, destY+dstH)

	if g.pipeline == nil {
		g.pipeline = video.New()
	}
	if err := g.pipeline.Draw(screen, source, srcRect, dstRect, g.pipelineOptions()); err != nil {
		// Un shader roto nunca debe impedir jugar: dibujamos el frame nativo.
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(float64(dstW)/float64(srcW), float64(dstH)/float64(srcH))
		op.GeoM.Translate(float64(destX), float64(destY))
		op.Filter = ebiten.FilterNearest
		screen.DrawImage(source.SubImage(srcRect).(*ebiten.Image), op)
		if g.osdTimer == 0 {
			g.showOSD("ERROR DE POSTPROCESADO: " + err.Error())
		}
	}

	if g.showHelp {
		g.drawHelp(screen)
	}

	// 3. Notificación OSD emergente al cambiar filtros (fondo translúcido + borde neón)
	if g.osdTimer > 0 {
		alpha := float32(1.0)
		if g.osdTimer < 25 {
			alpha = float32(g.osdTimer) / 25.0
		}
		badgeW := float32(len(g.osdText)*6 + 24)
		vector.DrawFilledRect(screen, 20, 20, badgeW, 26, color.RGBA{10, 15, 24, uint8(210 * alpha)}, true)
		vector.StrokeRect(screen, 20, 20, badgeW, 26, 1.5, color.RGBA{0, 160, 255, uint8(240 * alpha)}, true)
		ebitenutil.DebugPrintAt(screen, g.osdText, 30, 26)
	}
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	if g.state == StateMenu || g.state == StateLoading {
		return menuW, menuH
	}
	return outsideWidth, outsideHeight
}

// Variable global o de struct para la ruta actual de roms

func findRomFiles(dir string) []string {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".sms" || ext == ".gg" || ext == ".bin" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

func main() {
	scale := flag.Float64("scale", 3, "factor de escala de la ventana")
	fullscreen := flag.Bool("fullscreen", false, "arrancar a pantalla completa")
	crop := flag.Bool("crop", true, "recortar los 8 píxeles de overscan lateral izquierdo")
	romsDir := flag.String("roms", "roms", "directorio donde buscar juegos")
	hd := flag.Bool("hd", false, "arrancar con el pack HD activado")
	filter := flag.Int("filter", int(VideoFilterPixelPerfect), "filtro de vídeo inicial (0-5)")
	bench := flag.Int("bench", 0, "ejecuta N frames y sale midiendo FPS (requiere ROM)")
	benchWarmup := flag.Int("bench-warmup", 3600, "frames de calentamiento antes de medir con -bench")
	benchScript := flag.String("bench-script", "", "guion de entradas para -bench: frame:padhex:frames,...")
	flag.Parse()
	if *bench > 0 && flag.NArg() == 0 {
		log.Fatal("-bench requiere una ROM como argumento")
	}
	var benchPresses []benchPress
	benchDefault := false
	if *bench > 0 {
		benchPresses = parseBenchScript(*benchScript)
		if len(benchPresses) == 0 {
			benchPresses = defaultBenchScript()
			benchDefault = true
		}
	}
	initialFilter := VideoFilter(*filter)
	if initialFilter < 0 || int(initialFilter) >= len(videoFilterNames) {
		initialFilter = VideoFilterPixelPerfect
	}

	audioBuf := newAudioBuffer()
	audioCtx := audio.NewContext(sampleRate)
	player, err := audioCtx.NewPlayer(audioBuf)
	if err != nil {
		log.Fatalf("error al crear el reproductor de audio: %v", err)
	}
	player.SetVolume(1)
	player.Play()

	g := &game{
		state:        StateLoading,
		img:          ebiten.NewImage(vdp.ScreenW, vdp.ScreenH),
		fullscreen:   *fullscreen,
		crop:         *crop,
		audioBuf:     audioBuf,
		videoFilter:  initialFilter,
		audioFilter:  AudioFilterWarm, // Por defecto altavoz cálido analógico
		pipeline:     video.New(),
		hdMode:       *hd,
		benchFrames:  *bench,
		benchWarmup:  *benchWarmup,
		benchPresses: benchPresses,
		benchDefault: benchDefault,
		padNames:     make(map[ebiten.GamepadID]string),
	}

	if flag.NArg() > 0 {
		if err := g.loadGame(flag.Arg(0)); err != nil {
			log.Fatalf("error al cargar la ROM: %v", err)
		}
		g.state = StatePlaying
		g.curTitle = cleanTitle(flag.Arg(0))
		g.hdMode = *hd && machine.HD != nil && machine.HD.Active
	} else {
		cfg := loadConfig()
		*romsDir = cfg.RomsDir
		g.showHelp = !cfg.TutorialSeen
		g.roms = nil
		g.romFiles = findRomFiles(*romsDir)
		g.loadChan = make(chan RomItem, 50)
		g.loadDone = make(chan bool)
		if len(g.romFiles) > 0 {
			g.loadingMsg = cleanTitle(g.romFiles[0])
		}
		go func() {
			for _, path := range g.romFiles {
				thumb, err := generateSmartThumbnail(path)
				if err != nil {
					thumb = ebiten.NewImage(vdp.ScreenW, vdp.ScreenH)
					thumb.Fill(color.RGBA{20, 20, 30, 255})
				}
				g.loadChan <- RomItem{
					Path:      path,
					Title:     cleanTitle(path),
					Thumbnail: thumb,
				}
			}
			g.loadDone <- true
		}()
		ebiten.SetWindowTitle("gosms - Inicializando...")
	}

	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if *fullscreen {
		ebiten.SetFullscreen(true)
	}
	w := int(float64(vdp.ScreenW) * *scale)
	h := int(float64(vdp.ScreenH) * *scale)
	if g.state != StatePlaying {
		w = menuW
		h = menuH
	}
	ebiten.SetWindowSize(w, h)
	if *bench > 0 {
		ebiten.SetVsyncEnabled(false)
		ebiten.SetTPS(ebiten.SyncWithFPS)
	}

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
