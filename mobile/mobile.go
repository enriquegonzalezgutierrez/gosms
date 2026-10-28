package mobile

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"gosms/pkg/machine"
	"gosms/pkg/vdp"
	"gosms/pkg/video"
)

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
	m         *machine.Machine
	img       *ebiten.Image
	audioBuf  *audioBuffer
	monoBuf   []byte
	stereoBuf []byte
	touchIDs  []ebiten.TouchID
	pipeline  *video.Pipeline
	pauseHold bool
}

var globalGame *mobileGame

func newMobileGame() *mobileGame {
	aBuf := newAudioBuffer()
	audioCtx := audio.NewContext(sampleRate)
	player, err := audioCtx.NewPlayer(aBuf)
	if err == nil {
		player.SetVolume(1.0)
		player.Play()
	}

	// Por defecto inicializa sin ROM (esperando carga externa)
	g := &mobileGame{
		img:      ebiten.NewImage(vdp.ScreenW, vdp.ScreenH),
		audioBuf: aBuf,
		pipeline: video.New(),
	}
	globalGame = g
	return g
}

// LoadROM permite cargar una ROM desde la capa Java de Android
func LoadROM(data []byte) {
	if globalGame == nil {
		return
	}
	m := machine.New(data)
	m.CPU.PSG.SetSampleRate(sampleRate)
	globalGame.m = m
}

func (g *mobileGame) Update() error {
	if g.m == nil {
		return nil
	}

	pad := byte(0xFF)
	pausePressed := false

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

		dx := fx - dpadCX
		dy := fy - dpadCY
		dist := math.Hypot(dx, dy)
		if dist < dpadR*1.5 && dist > 15 {
			if dy < -dpadR*0.25 {
				pad &^= 0x01
			}
			if dy > dpadR*0.25 {
				pad &^= 0x02
			}
			if dx < -dpadR*0.25 {
				pad &^= 0x04
			}
			if dx > dpadR*0.25 {
				pad &^= 0x08
			}
		}

		if math.Hypot(fx-b1CX, fy-b1CY) < btnR*1.3 {
			pad &^= 0x10
		}
		if math.Hypot(fx-b2CX, fy-b2CY) < btnR*1.3 {
			pad &^= 0x20
		}
		if fx >= pauseX && fx <= pauseX+pauseW && fy >= pauseY && fy <= pauseY+pauseH {
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
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())

	if g.m == nil || g.m.VDP == nil {
		screen.Fill(color.RGBA{10, 15, 24, 255})
		msg := "GOSMS: Selecciona una ROM para jugar"
		ebitenutil.DebugPrintAt(screen, msg, int(sw)/2-len(msg)*3, int(sh)/2)
		return
	}

	g.img.WritePixels(g.m.VDP.Frame[:])
	srcRect := image.Rect(8, 0, vdp.ScreenW, vdp.ScreenH)
	srcW, srcH := float64(srcRect.Dx()), float64(srcRect.Dy())

	scale := math.Min(sw/srcW, sh/srcH)
	dstW := int(math.Round(srcW * scale))
	dstH := int(math.Round(srcH * scale))
	destX := (int(sw) - dstW) / 2
	destY := (int(sh) - dstH) / 2

	if g.pipeline == nil {
		g.pipeline = video.New()
	}
	if err := g.pipeline.Draw(screen, g.img, srcRect, image.Rect(destX, destY, destX+dstW, destY+dstH), video.DefaultOptions(video.PresetPixelPerfect, false)); err != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(destX), float64(destY))
		op.Filter = ebiten.FilterNearest
		screen.DrawImage(g.img.SubImage(srcRect).(*ebiten.Image), op)
	}

	// Controles táctiles
	ctrlAlpha := uint8(110)
	accentAlpha := uint8(160)

	dpadCX := float32(sw * 0.16)
	dpadCY := float32(sh * 0.72)
	dpadR := float32(sh * 0.22)

	vector.DrawFilledCircle(screen, dpadCX, dpadCY, dpadR, color.RGBA{15, 20, 30, ctrlAlpha}, true)
	vector.StrokeCircle(screen, dpadCX, dpadCY, dpadR, 2, color.RGBA{0, 140, 255, accentAlpha}, true)
	vector.DrawFilledCircle(screen, dpadCX, dpadCY, dpadR*0.35, color.RGBA{30, 40, 60, ctrlAlpha}, true)

	ebitenutil.DebugPrintAt(screen, "^", int(dpadCX)-3, int(dpadCY-dpadR*0.75))
	ebitenutil.DebugPrintAt(screen, "v", int(dpadCX)-3, int(dpadCY+dpadR*0.6))
	ebitenutil.DebugPrintAt(screen, "<", int(dpadCX-dpadR*0.75), int(dpadCY)-6)
	ebitenutil.DebugPrintAt(screen, ">", int(dpadCX+dpadR*0.6), int(dpadCY)-6)

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

func Dummy() {}
