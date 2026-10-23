// Command emulator arranca una Sega Master System con interfaz moderna,
// selector de ROMs y filtros de post-procesado de vídeo y audio en tiempo real.
// Desarrollado por Enrique González Gutiérrez.
package main

import (
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

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"alexkidd/pkg/machine"
	"alexkidd/pkg/vdp"
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
)

var videoFilterNames = []string{
	"PÍXEL PERFECT (NÍTIDO)",
	"CRT SCANLINES (TV DE TUBO)",
	"SCALE2X EPX (HD SUAVE)",
	"SUAVIZADO BILINEAL",
	"FÓSFORO VERDE MONOCROMO",
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
	var ids []ebiten.GamepadID
	ids = ebiten.AppendGamepadIDs(ids)
	if len(ids) > 0 {
		return inpututil.IsStandardGamepadButtonJustPressed(ids[0], b)
	}
	return false
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
	audioBuf   *audioBuffer
	dsp        AudioDSP
	monoBuf    []byte
	stereoBuf  []byte
	samplesBuf []int16
	drawOp     ebiten.DrawImageOptions
	frames     int

	// Filtros de post-procesado
	videoFilter VideoFilter
	audioFilter AudioFilter

	// Notificación en pantalla (OSD)
	osdText  string
	osdTimer int

	// Textura de scanlines CRT
	scanlineImg *ebiten.Image
	epxImg      *ebiten.Image
	epxBuf      []byte
}

func (g *game) loadGame(path string) error {
	rom, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	g.curRomPath = path
	g.m = machine.New(rom)
	machine.InitHD(path)
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

func (g *game) Update() error {
	g.frames++
	if g.osdTimer > 0 {
		g.osdTimer--
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		g.fullscreen = !g.fullscreen
		ebiten.SetFullscreen(g.fullscreen)
	}

	// Teclas rápidas globales para cambiar filtros
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
		g.videoFilter = (g.videoFilter + 1) % 5
		g.showOSD(fmt.Sprintf("VÍDEO: %s", videoFilterNames[g.videoFilter]))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) || inpututil.IsKeyJustPressed(ebiten.KeyB) {
		g.audioFilter = (g.audioFilter + 1) % 4
		g.showOSD(fmt.Sprintf("AUDIO: %s", audioFilterNames[g.audioFilter]))
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
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			return ebiten.Termination
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
			if inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonLeftRight) {
				g.selected = (g.selected + 1) % len(g.roms)
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonLeftLeft) {
				g.selected = (g.selected - 1 + len(g.roms)) % len(g.roms)
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) ||
				isGamepadButtonJustPressed(ebiten.StandardGamepadButtonRightBottom) {
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

		// Botón PAUSE físico de la consola (tecla P o botón Start del mando)
		if inpututil.IsKeyJustPressed(ebiten.KeyP) || isGamepadButtonJustPressed(ebiten.StandardGamepadButtonCenterRight) {
			g.m.CPU.NMI = true
			g.showOSD("BOTÓN PAUSE (NMI)")
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
		g.m.RunFrame()

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
	vector.DrawFilledRect(screen, 0, float32(sh)-52, float32(sw), 52, color.RGBA{9, 12, 18, 255}, true)
	vector.DrawFilledRect(screen, 0, float32(sh)-52, float32(sw), 1, color.RGBA{30, 42, 58, 255}, true)

	hint := "[ < / > ] Elegir   [ ENTER ] Jugar   [ R ] Carátula   [ F1 ] Vídeo   [ F2 ] Audio   [ F11 ] Pantalla Completa   [ ESC ] Salir"
	ebitenutil.DebugPrintAt(screen, hint, sw/2-len(hint)*3, sh-38)

	credit := fmt.Sprintf("gosms  ©  Creado por %s", authorName)
	ebitenutil.DebugPrintAt(screen, credit, sw/2-len(credit)*3, sh-18)
}

func (g *game) Draw(screen *ebiten.Image) {
	switch g.state {
	case StateLoading:
		g.drawLoading(screen)
		return

	case StateMenu:
		g.drawMenu(screen)
		return
	}

	if g.m == nil || g.m.VDP == nil {
		return
	}

	// 1. Cargar píxeles del VDP
	g.img.WritePixels(g.m.VDP.Frame[:])

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	var imgToDraw *ebiten.Image
	var srcW, srcH float64
	if g.crop {
		imgToDraw = g.img.SubImage(image.Rect(8, 0, vdp.ScreenW, vdp.ScreenH)).(*ebiten.Image)
		srcW, srcH = float64(vdp.ScreenW-8), float64(vdp.ScreenH)
	} else {
		imgToDraw = g.img
		srcW, srcH = float64(vdp.ScreenW), float64(vdp.ScreenH)
	}

	scale := math.Min(float64(sw)/srcW, float64(sh)/srcH)
	destX := (float64(sw) - srcW*scale) / 2
	destY := (float64(sh) - srcH*scale) / 2

	g.drawOp.GeoM.Reset()
	g.drawOp.GeoM.Scale(scale, scale)
	g.drawOp.GeoM.Translate(destX, destY)
	g.drawOp.ColorScale.Reset()

	// 2. Aplicar filtro de vídeo seleccionado
	switch g.videoFilter {
	case VideoFilterEPX:
		if g.epxImg == nil {
			g.epxImg = ebiten.NewImage(vdp.ScreenW*2, vdp.ScreenH*2)
			g.epxBuf = make([]byte, vdp.ScreenW*2*vdp.ScreenH*2*4)
		}
		// Algoritmo EPX / Scale2x por CPU
		src := g.m.VDP.Frame[:]
		dst := g.epxBuf
		srcW := vdp.ScreenW
		srcH := vdp.ScreenH
		for y := 0; y < srcH; y++ {
			for x := 0; x < srcW; x++ {
				// Índices centrales y vecinos
				idxP := (y*srcW + x) * 4
				var idxA, idxB, idxI, idxD int
				if y > 0 {
					idxA = ((y-1)*srcW + x) * 4
				} else {
					idxA = idxP
				}
				if y < srcH-1 {
					idxB = ((y+1)*srcW + x) * 4
				} else {
					idxB = idxP
				}
				if x > 0 {
					idxI = (y*srcW + x - 1) * 4
				} else {
					idxI = idxP
				}
				if x < srcW-1 {
					idxD = (y*srcW + x + 1) * 4
				} else {
					idxD = idxP
				}

				// Colores (leídos como uint32)
				p := uint32(src[idxP])<<16 | uint32(src[idxP+1])<<8 | uint32(src[idxP+2])
				a := uint32(src[idxA])<<16 | uint32(src[idxA+1])<<8 | uint32(src[idxA+2])
				b := uint32(src[idxB])<<16 | uint32(src[idxB+1])<<8 | uint32(src[idxB+2])
				i := uint32(src[idxI])<<16 | uint32(src[idxI+1])<<8 | uint32(src[idxI+2])
				d := uint32(src[idxD])<<16 | uint32(src[idxD+1])<<8 | uint32(src[idxD+2])

				// P1 P2
				// P3 P4
				p1, p2, p3, p4 := p, p, p, p

				if i == a && i != b && a != d {
					p1 = a
				}
				if a == d && a != i && d != b {
					p2 = a
				}
				if i == b && i != a && b != d {
					p3 = b
				}
				if b == d && b != i && d != a {
					p4 = b
				}

				// Escribir los 4 píxeles al buffer HD (2x)
				dIdx1 := ((y*2)*(srcW*2) + (x * 2)) * 4
				dIdx2 := dIdx1 + 4
				dIdx3 := ((y*2+1)*(srcW*2) + (x * 2)) * 4
				dIdx4 := dIdx3 + 4

				dst[dIdx1], dst[dIdx1+1], dst[dIdx1+2], dst[dIdx1+3] = byte(p1>>16), byte(p1>>8), byte(p1), 255
				dst[dIdx2], dst[dIdx2+1], dst[dIdx2+2], dst[dIdx2+3] = byte(p2>>16), byte(p2>>8), byte(p2), 255
				dst[dIdx3], dst[dIdx3+1], dst[dIdx3+2], dst[dIdx3+3] = byte(p3>>16), byte(p3>>8), byte(p3), 255
				dst[dIdx4], dst[dIdx4+1], dst[dIdx4+2], dst[dIdx4+3] = byte(p4>>16), byte(p4>>8), byte(p4), 255
			}
		}
		g.epxImg.WritePixels(g.epxBuf)

		// Recorte del overscan si está activado (para el imgToDraw 2x)
		var imgHD *ebiten.Image
		var drawW, drawH float64
		if g.crop {
			imgHD = g.epxImg.SubImage(image.Rect(16, 0, vdp.ScreenW*2, vdp.ScreenH*2)).(*ebiten.Image)
			drawW, drawH = float64(vdp.ScreenW*2-16), float64(vdp.ScreenH*2)
		} else {
			imgHD = g.epxImg
			drawW, drawH = float64(vdp.ScreenW*2), float64(vdp.ScreenH*2)
		}

		scaleHD := math.Min(float64(sw)/drawW, float64(sh)/drawH)
		destXHD := (float64(sw) - drawW*scaleHD) / 2
		destYHD := (float64(sh) - drawH*scaleHD) / 2

		g.drawOp.GeoM.Reset()
		g.drawOp.GeoM.Scale(scaleHD, scaleHD)
		g.drawOp.GeoM.Translate(destXHD, destYHD)
		g.drawOp.Filter = ebiten.FilterNearest
		screen.DrawImage(imgHD, &g.drawOp)

	case VideoFilterPixelPerfect:
		g.drawOp.Filter = ebiten.FilterNearest
		screen.DrawImage(imgToDraw, &g.drawOp)

	case VideoFilterSmooth:
		g.drawOp.Filter = ebiten.FilterLinear
		screen.DrawImage(imgToDraw, &g.drawOp)

	case VideoFilterMonochrome:
		// Estilo retro fósforo verde Game Boy
		g.drawOp.Filter = ebiten.FilterNearest
		g.drawOp.ColorScale.Scale(0.3, 1.2, 0.4, 1.0)
		screen.DrawImage(imgToDraw, &g.drawOp)

	case VideoFilterScanlines:
		g.drawOp.Filter = ebiten.FilterNearest
		screen.DrawImage(imgToDraw, &g.drawOp)

		// Rejilla de scanlines nítida adaptada a la escala de pantalla
		targetH := int(srcH * scale)
		if g.scanlineImg == nil || g.scanlineImg.Bounds().Dy() != targetH {
			g.scanlineImg = ebiten.NewImage(1, targetH)
			for y := 0; y < targetH; y++ {
				if y%3 == 2 { // Atenuación suave en la tercera línea de cada píxel
					g.scanlineImg.Set(0, y, color.RGBA{0, 0, 0, 80})
				}
			}
		}
		scanOp := &ebiten.DrawImageOptions{}
		scanOp.GeoM.Scale(srcW*scale, 1)
		scanOp.GeoM.Translate(destX, destY)
		screen.DrawImage(g.scanlineImg, scanOp)
	}

	if g.hdMode && machine.HD != nil && machine.HD.Active {
		g.drawHDOverlay(screen, scale, destX, destY)
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
	flag.Parse()

	audioBuf := newAudioBuffer()
	audioCtx := audio.NewContext(sampleRate)
	player, err := audioCtx.NewPlayer(audioBuf)
	if err != nil {
		log.Fatalf("error al crear el reproductor de audio: %v", err)
	}
	player.SetVolume(1)
	player.Play()

	g := &game{
		state:       StateLoading,
		img:         ebiten.NewImage(vdp.ScreenW, vdp.ScreenH),
		fullscreen:  *fullscreen,
		crop:        *crop,
		audioBuf:    audioBuf,
		videoFilter: VideoFilterPixelPerfect, // Por defecto arrancamos con CRT Scanlines para máxima autenticidad
		audioFilter: AudioFilterWarm,         // Por defecto altavoz cálido analógico
	}

	if flag.NArg() > 0 {
		if err := g.loadGame(flag.Arg(0)); err != nil {
			log.Fatalf("error al cargar la ROM: %v", err)
		}
		g.state = StatePlaying
		g.curTitle = cleanTitle(flag.Arg(0))
	} else {
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

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
