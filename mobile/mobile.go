package mobile

import (
	_ "embed"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/mobile"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"alexkidd/pkg/machine"
	"alexkidd/pkg/vdp"
)

//go:embed default.sms
var defaultROM []byte

const (
	sampleRate      = 48000
	samplesPerFrame = 801
)

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

	const maxBuffer = (sampleRate * 4 * 100) / 1000
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

type mobileGame struct {
	m          *machine.Machine
	img        *ebiten.Image
	audioBuf   *audioBuffer
	monoBuf    []byte
	samplesBuf []int16
	stereoBuf  []byte
	touchIDs   []ebiten.TouchID
	drawOp     ebiten.DrawImageOptions
	pauseHold  bool
}

func newmobileGame() *mobileGame {
	aBuf := newAudioBuffer()
	audioCtx := audio.NewContext(sampleRate)
	player, err := audioCtx.NewPlayer(aBuf)
	if err == nil {
		player.SetVolume(1.0)
		player.Play()
	}

	m := machine.New(defaultROM)
	m.CPU.PSG.SetSampleRate(sampleRate)

	return &mobileGame{
		m:        m,
		img:      ebiten.NewImage(vdp.ScreenW, vdp.ScreenH),
		audioBuf: aBuf,
	}
}

func (g *mobileGame) Update() error {
	sw, sh := float64(g.img.Bounds().Dx()), float64(g.img.Bounds().Dy())
	_ = sw
	_ = sh

	pad := byte(0xFF)
	pausePressed := false

	// --- Lectura Táctil ---
	g.touchIDs = ebiten.AppendTouchIDs(g.touchIDs[:0])
	ww, wh := ebiten.WindowSize()
	if ww == 0 || wh == 0 {
		ww, wh = 800, 480
	}
	fW, fH := float64(ww), float64(wh)

	dpadCX := fW * 0.16
	dpadCY := fH * 0.72
	dpadR := fH * 0.22

	b1CX := fW - fH*0.30
	b1CY := fH * 0.76
	b2CX := fW - fH*0.14
	b2CY := fH * 0.60
	btnR := fH * 0.12

	pauseX := fW*0.5 - 50
	pauseY := 10.0
	pauseW := 100.0
	pauseH := 40.0

	for _, id := range g.touchIDs {
		x, y := ebiten.TouchPosition(id)
		fx, fy := float64(x), float64(y)

		// D-Pad
		dx := fx - dpadCX
		dy := fy - dpadCY
		dist := math.Hypot(dx, dy)
		if dist < dpadR*1.5 && dist > 15 {
			if dy < -dpadR*0.25 {
				pad &^= 0x01 // Arriba
			}
			if dy > dpadR*0.25 {
				pad &^= 0x02 // Abajo
			}
			if dx < -dpadR*0.25 {
				pad &^= 0x04 // Izquierda
			}
			if dx > dpadR*0.25 {
				pad &^= 0x08 // Derecha
			}
		}

		// Botón 1
		if math.Hypot(fx-b1CX, fy-b1CY) < btnR*1.3 {
			pad &^= 0x10
		}
		// Botón 2
		if math.Hypot(fx-b2CX, fy-b2CY) < btnR*1.3 {
			pad &^= 0x20
		}

		// Botón PAUSE
		if fx >= pauseX && fx <= pauseX+pauseW && fy >= pauseY && fy <= pauseY+pauseH {
			pausePressed = true
		}
	}

	// --- Mandos Físicos (Bluetooth / USB OTG) ---
	var ids []ebiten.GamepadID
	ids = ebiten.AppendGamepadIDs(ids)
	if len(ids) > 0 {
		gid := ids[0]
		if ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonLeftTop) ||
			ebiten.StandardGamepadAxisValue(gid, ebiten.StandardGamepadAxisLeftStickVertical) < -0.4 {
			pad &^= 0x01
		}
		if ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonLeftBottom) ||
			ebiten.StandardGamepadAxisValue(gid, ebiten.StandardGamepadAxisLeftStickVertical) > 0.4 {
			pad &^= 0x02
		}
		if ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonLeftLeft) ||
			ebiten.StandardGamepadAxisValue(gid, ebiten.StandardGamepadAxisLeftStickHorizontal) < -0.4 {
			pad &^= 0x04
		}
		if ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonLeftRight) ||
			ebiten.StandardGamepadAxisValue(gid, ebiten.StandardGamepadAxisLeftStickHorizontal) > 0.4 {
			pad &^= 0x08
		}
		if ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonRightBottom) ||
			ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonRightLeft) {
			pad &^= 0x10
		}
		if ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonRightRight) ||
			ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonRightTop) {
			pad &^= 0x20
		}
		if ebiten.IsStandardGamepadButtonPressed(gid, ebiten.StandardGamepadButtonCenterRight) {
			pausePressed = true
		}
	}

	if pausePressed && !g.pauseHold {
		g.m.CPU.NMI = true
		g.pauseHold = true
	} else if !pausePressed {
		g.pauseHold = false
	}

	g.m.CPU.Pad1 = pad
	g.m.RunFrame()

	// Audio
	monoBytes := samplesPerFrame * 2
	if len(g.monoBuf) < monoBytes {
		g.monoBuf = make([]byte, monoBytes)
	}
	g.m.PSG.Render(g.monoBuf[:monoBytes])

	stereoBytes := samplesPerFrame * 4
	if len(g.stereoBuf) < stereoBytes {
		g.stereoBuf = make([]byte, stereoBytes)
	}
	for i := 0; i < samplesPerFrame; i++ {
		b0 := g.monoBuf[i*2]
		b1 := g.monoBuf[i*2+1]
		g.stereoBuf[i*4] = b0
		g.stereoBuf[i*4+1] = b1
		g.stereoBuf[i*4+2] = b0
		g.stereoBuf[i*4+3] = b1
	}
	g.audioBuf.Write(g.stereoBuf[:stereoBytes])

	return nil
}

func (g *mobileGame) Draw(screen *ebiten.Image) {
	if g.m == nil || g.m.VDP == nil {
		return
	}

	g.img.WritePixels(g.m.VDP.Frame[:])

	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
	srcW, srcH := float64(vdp.ScreenW-8), float64(vdp.ScreenH) // Recorte de overscan 8px
	subImg := g.img.SubImage(image.Rect(8, 0, vdp.ScreenW, vdp.ScreenH)).(*ebiten.Image)

	scale := math.Min(sw/srcW, sh/srcH)
	destX := (sw - srcW*scale) / 2
	destY := (sh - srcH*scale) / 2

	g.drawOp.GeoM.Reset()
	g.drawOp.GeoM.Scale(scale, scale)
	g.drawOp.GeoM.Translate(destX, destY)
	g.drawOp.Filter = ebiten.FilterNearest
	screen.DrawImage(subImg, &g.drawOp)

	// --- Renderizar Controles Virtuales Superpuestos ---
	ctrlAlpha := uint8(110)
	accentAlpha := uint8(160)

	dpadCX := float32(sw * 0.16)
	dpadCY := float32(sh * 0.72)
	dpadR := float32(sh * 0.22)

	// Base D-Pad
	vector.DrawFilledCircle(screen, dpadCX, dpadCY, dpadR, color.RGBA{15, 20, 30, ctrlAlpha}, true)
	vector.StrokeCircle(screen, dpadCX, dpadCY, dpadR, 2, color.RGBA{0, 140, 255, accentAlpha}, true)
	// Centro D-Pad
	vector.DrawFilledCircle(screen, dpadCX, dpadCY, dpadR*0.35, color.RGBA{30, 40, 60, ctrlAlpha}, true)

	// Flechas D-Pad
	ebitenutil.DebugPrintAt(screen, "^", int(dpadCX)-3, int(dpadCY-dpadR*0.75))
	ebitenutil.DebugPrintAt(screen, "v", int(dpadCX)-3, int(dpadCY+dpadR*0.6))
	ebitenutil.DebugPrintAt(screen, "<", int(dpadCX-dpadR*0.75), int(dpadCY)-6)
	ebitenutil.DebugPrintAt(screen, ">", int(dpadCX+dpadR*0.6), int(dpadCY)-6)

	// Botones 1 y 2
	b1CX := float32(sw - sh*0.30)
	b1CY := float32(sh * 0.76)
	b2CX := float32(sw - sh*0.14)
	b2CY := float32(sh * 0.60)
	btnR := float32(sh * 0.12)

	vector.DrawFilledCircle(screen, b1CX, b1CY, btnR, color.RGBA{15, 20, 30, ctrlAlpha}, true)
	vector.StrokeCircle(screen, b1CX, b1CY, btnR, 2.5, color.RGBA{0, 200, 100, accentAlpha}, true)
	ebitenutil.DebugPrintAt(screen, "1", int(b1CX)-3, int(b1CY)-6)

	vector.DrawFilledCircle(screen, b2CX, b2CY, btnR, color.RGBA{15, 20, 30, ctrlAlpha}, true)
	vector.StrokeCircle(screen, b2CX, b2CY, btnR, 2.5, color.RGBA{255, 60, 60, accentAlpha}, true)
	ebitenutil.DebugPrintAt(screen, "2", int(b2CX)-3, int(b2CY)-6)

	// Botón Pause
	pauseX := float32(sw*0.5 - 50)
	pauseY := float32(10)
	pauseW := float32(100)
	pauseH := float32(36)
	vector.DrawFilledRect(screen, pauseX, pauseY, pauseW, pauseH, color.RGBA{15, 20, 30, ctrlAlpha}, true)
	vector.StrokeRect(screen, pauseX, pauseY, pauseW, pauseH, 1.5, color.RGBA{220, 180, 0, accentAlpha}, true)
	ebitenutil.DebugPrintAt(screen, "PAUSE", int(pauseX)+30, int(pauseY)+10)
}

func (g *mobileGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func init() {
	game := newmobileGame()
	mobile.SetGame(game)
}

// Dummy exportado para asegurar el enlace por gomobile / gobind
func Dummy() {}
