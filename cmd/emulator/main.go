package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"alexkidd/pkg/machine"
	"alexkidd/pkg/vdp"
)

// Frecuencia de muestreo del audio.
const sampleRate = 48000

// ---------- Audio ----------

// audioBuffer es una cola de muestras PCM sincronizada con el bucle de emulación.
// audioBuffer es una cola FIFO con señalización thread-safe para alimentar a Ebiten.
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

	// Si el buffer acumula más de 100 ms (~6 frames), descartamos lo viejo para evitar lag
	const maxBuffer = (sampleRate * 4 * 100) / 1000 // 19.200 bytes
	if len(a.buf) > maxBuffer {
		a.buf = a.buf[len(a.buf)-maxBuffer:]
	}

	a.buf = append(a.buf, p...)
	a.cond.Signal() // Despertar al lector de audio
}

func (a *audioBuffer) Read(p []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Esperar hasta que haya muestras disponibles producidas por la emulación
	for len(a.buf) == 0 {
		a.cond.Wait()
	}

	n := copy(p, a.buf)
	a.buf = a.buf[n:]
	return n, nil // Devolver SOLO los bytes reales leídos
}

// ---------- Entrada ----------

type keyBit struct {
	key ebiten.Key
	bit byte
}

// El mando de la SMS es activo-bajo: bit=0 significa pulsado.
var pad1Keys = []keyBit{
	{ebiten.KeyArrowUp, 0x01},
	{ebiten.KeyArrowDown, 0x02},
	{ebiten.KeyArrowLeft, 0x04},
	{ebiten.KeyArrowRight, 0x08},
	{ebiten.KeyZ, 0x10},
	{ebiten.KeySpace, 0x10},
	{ebiten.KeyX, 0x20},
	{ebiten.KeyEnter, 0x20},
}

var pad2Keys = []keyBit{
	{ebiten.KeyW, 0x01},
	{ebiten.KeyS, 0x02},
	{ebiten.KeyA, 0x04},
	{ebiten.KeyD, 0x08},
	{ebiten.KeyQ, 0x10},
	{ebiten.KeyE, 0x20},
}

func readPad(keys []keyBit) byte {
	v := byte(0xFF)
	for _, kb := range keys {
		if ebiten.IsKeyPressed(kb.key) {
			v &^= kb.bit
		}
	}
	return v
}

// ---------- Juego ----------

type game struct {
	m          *machine.Machine
	img        *ebiten.Image
	fullscreen bool
	stats      bool
	frames     int
	audioBuf   *audioBuffer
	monoBuf    []byte
	stereoBuf  []byte
	crop       bool
	drawOp     ebiten.DrawImageOptions
}

func (g *game) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		g.fullscreen = !g.fullscreen
		ebiten.SetFullscreen(g.fullscreen)
	}

	g.m.CPU.Pad1 = readPad(pad1Keys)
	g.m.CPU.Pad2 = readPad(pad2Keys)

	g.m.RunFrame()

	// Master System NTSC: 3579545 / (228 * 262) = 59.9227 Hz -> 48000 / 59.9227 = 801 muestras/frame
	const samplesPerFrame = 801
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
		lo := g.monoBuf[i*2]
		hi := g.monoBuf[i*2+1]
		g.stereoBuf[i*4+0] = lo
		g.stereoBuf[i*4+1] = hi
		g.stereoBuf[i*4+2] = lo
		g.stereoBuf[i*4+3] = hi
	}
	g.audioBuf.Write(g.stereoBuf[:stereoBytes])

	// Rendimiento: sirve para comprobar que la emulación va a 60 Hz (si va más
	// lenta, la música también se reproduce más lenta, porque el motor de sonido
	// del juego avanza con la interrupción de frame).
	g.frames++
	if g.frames%30 == 0 {
		ebiten.SetWindowTitle(fmt.Sprintf("Alex Kidd - Sega Master System   %.0f fps  %.1f TPS",
			ebiten.ActualFPS(), ebiten.ActualTPS()))
	}
	if g.stats && g.frames%60 == 0 {
		fmt.Fprintf(os.Stderr, "frame %d  fps=%.1f  tps=%.1f\n", g.frames, ebiten.ActualFPS(), ebiten.ActualTPS())
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
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
	g.drawOp.GeoM.Reset()
	g.drawOp.GeoM.Scale(scale, scale)
	g.drawOp.GeoM.Translate((float64(sw)-srcW*scale)/2, (float64(sh)-srcH*scale)/2)
	g.drawOp.Filter = ebiten.FilterNearest
	screen.DrawImage(imgToDraw, &g.drawOp)
}

// Layout devuelve el tamaño real de la ventana para que la imagen se reescale
// al redimensionar.
func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	scale := flag.Float64("scale", 3, "factor de escala de la ventana")
	fullscreen := flag.Bool("fullscreen", false, "arrancar a pantalla completa")
	skip := flag.Int("skip", 0, "emular N frames antes de mostrar la ventana (para saltar a un punto concreto)")
	stats := flag.Bool("stats", false, "escribir FPS/TPS en stderr una vez por segundo")
	crop := flag.Bool("crop", false, "recortar los 8 píxeles de overscan lateral izquierdo")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Uso: emulator [-scale N] [-fullscreen] [-skip N] [-stats] <juego.sms>")
		os.Exit(1)
	}

	rom, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatalf("error al cargar la ROM: %v", err)
	}

	m := machine.New(rom)
	m.CPU.PSG.SetSampleRate(sampleRate)

	// Avance rápido inicial: útil para saltar directamente a una escena.
	for i := 0; i < *skip; i++ {
		m.RunFrame()
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
		m:          m,
		img:        ebiten.NewImage(vdp.ScreenW, vdp.ScreenH),
		fullscreen: *fullscreen,
		stats:      *stats,
		audioBuf:   audioBuf,
		crop:       *crop,
	}

	ebiten.SetWindowTitle("Alex Kidd - Sega Master System")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if *fullscreen {
		ebiten.SetFullscreen(true)
	}
	w := int(float64(vdp.ScreenW) * *scale)
	h := int(float64(vdp.ScreenH) * *scale)
	ebiten.SetWindowSize(w, h)
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
