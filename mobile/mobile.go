package mobile

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"strings"
	"sync"

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
	sampleRate      = 48000
	samplesPerFrame = 801
)

// Picker is implemented by MainActivity. Go calls PickROM when the launcher
// asks for a ROM; the Java side opens the Storage Access Framework picker.
type Picker interface {
	PickROM()
}

var (
	pickerMu     sync.Mutex
	globalPicker Picker
)

// SetPicker connects the Android activity to the launcher.
func SetPicker(p Picker) {
	pickerMu.Lock()
	globalPicker = p
	pickerMu.Unlock()
}

func requestPicker() {
	pickerMu.Lock()
	p := globalPicker
	pickerMu.Unlock()
	if p != nil {
		p.PickROM()
	}
}

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

type mobileState int

const (
	stateLauncher mobileState = iota
	statePlaying
)

type mobileGame struct {
	mu sync.Mutex

	m       *machine.Machine
	state   mobileState
	romName string
	romData []byte

	img       *ebiten.Image
	audioBuf  *audioBuffer
	monoBuf   []byte
	stereoBuf []byte
	pipeline  *video.Pipeline

	touchIDs    []ebiten.TouchID
	seenTouches map[ebiten.TouchID]bool
	pauseHold   bool
	hudPad      byte
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

	g := &mobileGame{
		img:         ebiten.NewImage(vdp.ScreenW, vdp.ScreenH),
		audioBuf:    aBuf,
		pipeline:    video.New(),
		state:       stateLauncher,
		seenTouches: make(map[ebiten.TouchID]bool),
	}
	globalGame = g
	return g
}

func snapshot(g *mobileGame) (*machine.Machine, mobileState, string, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.m, g.state, g.romName, len(g.romData)
}

// SetROM stores a picked ROM and leaves the launcher visible. The user taps
// JUGAR, or the Java picker calls Play right after picking for a direct start.
func SetROM(name string, data []byte) {
	if len(data) < 1024 || globalGame == nil {
		return
	}
	copied := append([]byte(nil), data...)
	var m *machine.Machine
	func() {
		defer func() {
			if recover() != nil {
				m = nil
			}
		}()
		m = machine.New(copied)
	}()
	if m == nil {
		return
	}
	m.CPU.PSG.SetSampleRate(sampleRate)

	globalGame.mu.Lock()
	globalGame.m = m
	globalGame.romName = cleanROMName(name)
	globalGame.romData = copied
	globalGame.state = stateLauncher
	globalGame.mu.Unlock()
}

// LoadROM mantiene la API antigua para clientes que solo pasan bytes.
func LoadROM(data []byte) {
	SetROM("ROM", data)
	Play()
}

// Play starts or resumes the loaded ROM.
func Play() {
	if globalGame == nil {
		return
	}
	globalGame.mu.Lock()
	if globalGame.m != nil {
		globalGame.state = statePlaying
	}
	globalGame.mu.Unlock()
}

// ShowLauncher returns to the ROM card/game menu.
func ShowLauncher() {
	if globalGame == nil {
		return
	}
	globalGame.mu.Lock()
	globalGame.state = stateLauncher
	globalGame.mu.Unlock()
}

// HandleBack is called by MainActivity. It returns true when the app should
// exit; from the game it only returns to the launcher.
func HandleBack() bool {
	if globalGame == nil {
		return true
	}
	globalGame.mu.Lock()
	defer globalGame.mu.Unlock()
	if globalGame.state == statePlaying {
		globalGame.state = stateLauncher
		return false
	}
	return true
}

func cleanROMName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.TrimSpace(name)
	if name == "" {
		return "ROM"
	}
	return name
}

func shortName(name string, limit int) string {
	runes := []rune(name)
	if len(runes) <= limit {
		return name
	}
	return string(runes[:limit-3]) + "..."
}

func (g *mobileGame) Update() error {
	m, state, _, _ := snapshot(g)
	if state != statePlaying || m == nil {
		g.updateLauncher()
		return nil
	}

	pad := byte(0xFF)
	pausePressed := false
	menuPressed := false

	ww, wh := ebiten.WindowSize()
	if ww == 0 || wh == 0 {
		ww, wh = 800, 480
	}
	fW, fH := float64(ww), float64(wh)
	hud := makeHUD(fW, fH)

	g.touchIDs = ebiten.AppendTouchIDs(g.touchIDs[:0])
	for _, id := range g.touchIDs {
		x, y := ebiten.TouchPosition(id)
		fx, fy := float64(x), float64(y)

		for _, dir := range hud.directions() {
			if math.Hypot(fx-dir.x, fy-dir.y) < dir.r*1.25 {
				pad &^= dir.bit
			}
		}
		if math.Hypot(fx-hud.b1X, fy-hud.b1Y) < hud.btnR*1.25 {
			pad &^= 0x10
		}
		if math.Hypot(fx-hud.b2X, fy-hud.b2Y) < hud.btnR*1.25 {
			pad &^= 0x20
		}
		if hud.pause.hit(fx, fy) {
			pausePressed = true
		}
		if hud.menu.hit(fx, fy) {
			menuPressed = true
		}
	}

	if menuPressed {
		ShowLauncher()
		return nil
	}
	if pausePressed && !g.pauseHold {
		m.CPU.NMI = true
		g.pauseHold = true
	} else if !pausePressed {
		g.pauseHold = false
	}

	g.hudPad = pad
	m.CPU.Pad1 = pad
	m.RunFrame()

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

func (g *mobileGame) updateLauncher() {
	g.touchIDs = ebiten.AppendTouchIDs(g.touchIDs[:0])
	seen := make(map[ebiten.TouchID]bool, len(g.touchIDs))
	for _, id := range g.touchIDs {
		seen[id] = true
		if g.seenTouches[id] {
			continue
		}
		g.seenTouches[id] = true
		x, y := ebiten.TouchPosition(id)
		g.tapLauncher(float64(x), float64(y))
	}
	for id := range g.seenTouches {
		if !seen[id] {
			delete(g.seenTouches, id)
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		g.tapLauncher(float64(x), float64(y))
	}
}

func (g *mobileGame) tapLauncher(x, y float64) {
	ww, wh := ebiten.WindowSize()
	if ww == 0 || wh == 0 {
		ww, wh = 800, 480
	}
	lay := makeLauncher(float64(ww), float64(wh), g.hasROM())
	switch {
	case lay.choose.hit(x, y), lay.change.hit(x, y):
		requestPicker()
	case lay.play.hit(x, y):
		Play()
	}
}

func (g *mobileGame) hasROM() bool {
	m, _, _, _ := snapshot(g)
	return m != nil
}

func (g *mobileGame) Draw(screen *ebiten.Image) {
	m, state, name, size := snapshot(g)
	if state != statePlaying || m == nil {
		g.drawLauncher(screen, name, size)
		return
	}

	screen.Fill(color.RGBA{0, 0, 0, 255})
	g.img.WritePixels(m.VDP.Frame[:])
	srcRect := image.Rect(8, 0, vdp.ScreenW, vdp.ScreenH)
	srcW, srcH := float64(srcRect.Dx()), float64(srcRect.Dy())
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())

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

	g.drawHUD(screen, sw, sh, g.hudPad)
}

func (g *mobileGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

// ---------------------------------------------------------------------------
// Dibujo de UI
// ---------------------------------------------------------------------------

type box struct {
	x, y, w, h float64
}

func (b box) hit(x, y float64) bool {
	return x >= b.x && x <= b.x+b.w && y >= b.y && y <= b.y+b.h
}

var textCache = map[string]*ebiten.Image{}

func textImage(text string) *ebiten.Image {
	if img, ok := textCache[text]; ok {
		return img
	}
	w := len(text)*6 + 4
	if w < 4 {
		w = 4
	}
	img := ebiten.NewImage(w, 16)
	ebitenutil.DebugPrintAt(img, text, 1, 0)
	textCache[text] = img
	return img
}

func drawText(dst *ebiten.Image, text string, x, y, scale float64, c color.RGBA) {
	if text == "" || scale <= 0 {
		return
	}
	img := textImage(text)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x, y)
	op.ColorScale.Scale(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
	dst.DrawImage(img, op)
}

func drawTextCentered(dst *ebiten.Image, text string, cx, y, scale float64, c color.RGBA) {
	w := float64(len(text)) * 6 * scale
	drawText(dst, text, cx-w/2, y, scale, c)
}

func drawButton(dst *ebiten.Image, b box, label string, scale float64, accent color.RGBA, primary bool) {
	fill := color.RGBA{16, 22, 34, 235}
	if primary {
		fill = color.RGBA{0, 70, 150, 235}
	}
	vector.DrawFilledRect(dst, float32(b.x), float32(b.y), float32(b.w), float32(b.h), fill, true)
	vector.StrokeRect(dst, float32(b.x), float32(b.y), float32(b.w), float32(b.h), float32(math.Max(1, scale*1.2)), accent, true)
	c := color.RGBA{235, 244, 255, 255}
	drawTextCentered(dst, label, b.x+b.w/2, b.y+(b.h-16*scale)/2, scale, c)
}

type hudLayout struct {
	scale                    float64
	dpadCX, dpadCY, dpadR    float64
	dpadArm, dpadBtnR        float64
	b1X, b1Y, b2X, b2Y, btnR float64
	menu, pause              box
}

type direction struct {
	x, y, r float64
	bit     byte
	label   string
}

func makeHUD(w, h float64) hudLayout {
	s := math.Min(w/800, h/480)
	if s < 0.8 {
		s = 0.8
	}
	r := 96 * s
	cx := 34*s + r
	cy := h - 30*s - r
	br := 44 * s
	return hudLayout{
		scale:  s,
		dpadCX: cx, dpadCY: cy, dpadR: r,
		dpadArm: r * 0.55, dpadBtnR: r * 0.34,
		b1X: w - 42*s - br, b1Y: h - 44*s - br,
		b2X: w - 42*s - br - 112*s, b2Y: h - 44*s - br - 62*s,
		btnR:  br,
		menu:  box{x: 18 * s, y: 14 * s, w: 110 * s, h: 38 * s},
		pause: box{x: w/2 - 70*s, y: 14 * s, w: 140 * s, h: 38 * s},
	}
}

func (h hudLayout) directions() []direction {
	return []direction{
		{x: h.dpadCX, y: h.dpadCY - h.dpadArm, r: h.dpadBtnR, bit: 0x01, label: "^"},
		{x: h.dpadCX, y: h.dpadCY + h.dpadArm, r: h.dpadBtnR, bit: 0x02, label: "v"},
		{x: h.dpadCX - h.dpadArm, y: h.dpadCY, r: h.dpadBtnR, bit: 0x04, label: "<"},
		{x: h.dpadCX + h.dpadArm, y: h.dpadCY, r: h.dpadBtnR, bit: 0x08, label: ">"},
	}
}

func (g *mobileGame) padStatus() string {
	ids := ebiten.AppendGamepadIDs(nil)
	if len(ids) == 0 {
		return "MANDO: ninguno"
	}
	name := strings.TrimSpace(ebiten.GamepadName(ids[0]))
	if name == "" {
		name = "Mando USB/Bluetooth"
	}
	if len(ids) == 1 {
		return "MANDO: " + shortName(name, 36)
	}
	return "MANDOS: " + shortName(name, 30)
}

func (g *mobileGame) drawHUD(dst *ebiten.Image, w, h float64, pad byte) {
	hud := makeHUD(w, h)
	s := hud.scale

	// Barra superior translucida.
	vector.DrawFilledRect(dst, 0, 0, float32(w), float32(58*s), color.RGBA{8, 12, 19, 170}, true)
	drawButton(dst, hud.menu, "MENU", 1.15*s, color.RGBA{0, 180, 255, 220}, false)
	drawButton(dst, hud.pause, "PAUSA", 1.15*s, color.RGBA{230, 190, 40, 220}, false)

	// Base del D-pad.
	vector.DrawFilledCircle(dst, float32(hud.dpadCX), float32(hud.dpadCY), float32(hud.dpadR*0.95), color.RGBA{10, 15, 24, 90}, true)
	vector.StrokeCircle(dst, float32(hud.dpadCX), float32(hud.dpadCY), float32(hud.dpadR*0.95), float32(1.5*s), color.RGBA{0, 140, 255, 100}, true)
	for _, d := range hud.directions() {
		pressed := pad&d.bit == 0
		fill := color.RGBA{12, 18, 28, 150}
		stroke := color.RGBA{0, 150, 255, 190}
		if pressed {
			fill = color.RGBA{0, 110, 210, 210}
			stroke = color.RGBA{170, 235, 255, 255}
		}
		vector.DrawFilledCircle(dst, float32(d.x), float32(d.y), float32(d.r), fill, true)
		vector.StrokeCircle(dst, float32(d.x), float32(d.y), float32(d.r), float32(1.4*s), stroke, true)
		drawTextCentered(dst, d.label, d.x, d.y-8*1.4*s, 1.4*s, color.RGBA{235, 245, 255, 230})
	}
	vector.DrawFilledCircle(dst, float32(hud.dpadCX), float32(hud.dpadCY), float32(hud.dpadR*0.22), color.RGBA{20, 30, 45, 180}, true)

	drawCircleButton := func(cx, cy, r float64, label string, accent color.RGBA, pressed bool) {
		fill := color.RGBA{12, 18, 28, 155}
		if pressed {
			fill = color.RGBA{20, 70, 50, 220}
		}
		vector.DrawFilledCircle(dst, float32(cx), float32(cy), float32(r), fill, true)
		vector.StrokeCircle(dst, float32(cx), float32(cy), float32(r), float32(1.8*s), accent, true)
		drawTextCentered(dst, label, cx, cy-8*1.6*s, 1.6*s, color.RGBA{240, 248, 255, 240})
	}
	drawCircleButton(hud.b1X, hud.b1Y, hud.btnR, "1", color.RGBA{0, 210, 120, 230}, pad&0x10 == 0)
	drawCircleButton(hud.b2X, hud.b2Y, hud.btnR, "2", color.RGBA{255, 70, 70, 230}, pad&0x20 == 0)

	rom := shortName("ROM", 30)
	_, _, name, _ := snapshot(g)
	if name != "" {
		rom = shortName(name, 30)
	}
	drawText(dst, rom, w-24*s-float64(len(rom))*6*0.9*s, 62*s, 0.9*s, color.RGBA{150, 190, 225, 220})
}

type launcherLayout struct {
	scale                float64
	card                 box
	choose, play, change box
}

func makeLauncher(w, h float64, hasROM bool) launcherLayout {
	s := math.Min(w/800, h/480)
	if s < 0.8 {
		s = 0.8
	}
	cardW := math.Min(w-80*s, 700*s)
	cardH := 280 * s
	cardX := (w - cardW) / 2
	cardY := math.Max(150*s, h/2-cardH/2)
	btnH := 54 * s
	btnW := 230 * s
	lay := launcherLayout{
		scale: s,
		card:  box{cardX, cardY, cardW, cardH},
	}
	if hasROM {
		lay.play = box{cardX + cardW/2 - btnW - 12*s, cardY + cardH - btnH - 28*s, btnW, btnH}
		lay.change = box{cardX + cardW/2 + 12*s, cardY + cardH - btnH - 28*s, btnW, btnH}
	} else {
		lay.choose = box{cardX + cardW/2 - btnW/2, cardY + cardH - btnH - 28*s, btnW, btnH}
	}
	return lay
}

func (g *mobileGame) drawLauncher(dst *ebiten.Image, romName string, romSize int) {
	sw, sh := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	dst.Fill(color.RGBA{11, 15, 21, 255})
	vector.DrawFilledRect(dst, 0, 0, float32(sw), float32(sh/2), color.RGBA{15, 21, 32, 255}, false)

	lay := makeLauncher(sw, sh, romName != "")
	s := lay.scale

	// Cabecera.
	vector.DrawFilledRect(dst, 0, 0, float32(sw), float32(64*s), color.RGBA{8, 11, 17, 255}, true)
	vector.DrawFilledRect(dst, 0, float32(64*s), float32(sw), float32(2*s), color.RGBA{0, 140, 255, 255}, true)
	vector.DrawFilledRect(dst, float32(24*s), float32(15*s), float32(74*s), float32(32*s), color.RGBA{0, 85, 204, 255}, true)
	drawText(dst, "SEGA", 30*s, 18*s, 1.35*s, color.RGBA{235, 245, 255, 255})
	drawText(dst, "GOSMS  ·  MASTER SYSTEM", 112*s, 19*s, 1.15*s, color.RGBA{225, 238, 250, 255})
	drawText(dst, g.padStatus(), 24*s, 72*s, 0.95*s, color.RGBA{130, 180, 220, 230})

	// Tarjeta central.
	drawTextCentered(dst, "SELECCIONA TU JUEGO", sw/2, 105*s, 1.9*s, color.RGBA{240, 246, 255, 255})
	vector.DrawFilledRect(dst, float32(lay.card.x), float32(lay.card.y), float32(lay.card.w), float32(lay.card.h), color.RGBA{17, 24, 37, 245}, true)
	vector.StrokeRect(dst, float32(lay.card.x), float32(lay.card.y), float32(lay.card.w), float32(lay.card.h), float32(math.Max(1.5, 1.5*s)), color.RGBA{0, 150, 255, 220}, true)

	if romName == "" {
		drawTextCentered(dst, "SIN ROM", sw/2, lay.card.y+55*s, 2.2*s, color.RGBA{160, 190, 220, 230})
		drawTextCentered(dst, "Elige un archivo .sms de tu dispositivo", sw/2, lay.card.y+105*s, 0.95*s, color.RGBA{150, 175, 205, 220})
		drawTextCentered(dst, "El selector del sistema abrirá tus archivos", sw/2, lay.card.y+130*s, 0.85*s, color.RGBA{110, 140, 175, 220})
	} else {
		drawTextCentered(dst, shortName(romName, 34), sw/2, lay.card.y+52*s, 1.75*s, color.RGBA{240, 246, 255, 255})
		drawTextCentered(dst, "ROM cargada · "+formatKB(romSize), sw/2, lay.card.y+94*s, 0.95*s, color.RGBA{150, 185, 220, 220})
		drawTextCentered(dst, "Pulsa JUGAR para empezar", sw/2, lay.card.y+126*s, 0.9*s, color.RGBA{120, 155, 195, 220})
	}

	if romName == "" {
		drawButton(dst, lay.choose, "ELEGIR ROM", 1.25*s, color.RGBA{0, 160, 255, 230}, true)
	} else {
		drawButton(dst, lay.play, "JUGAR", 1.35*s, color.RGBA{0, 210, 130, 230}, true)
		drawButton(dst, lay.change, "CAMBIAR ROM", 1.15*s, color.RGBA{0, 160, 255, 230}, false)
	}

	// Pie.
	drawTextCentered(dst, "Atrás: salir   ·   Mando USB/Bluetooth: detección automática", sw/2, sh-30*s, 0.8*s, color.RGBA{95, 125, 160, 220})
	drawTextCentered(dst, "GoSMS · Sega Master System", sw/2, sh-14*s, 0.7*s, color.RGBA{65, 90, 120, 220})
}

func formatKB(n int) string {
	if n <= 0 {
		return "0 KB"
	}
	if n < 1024*1024 {
		return fmtInt(n/1024) + " KB"
	}
	return fmtInt(n/(1024*1024)) + " MB"
}

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}

func Dummy() {}
