// Command emulator arranca una Sega Master System en una ventana con Ebiten.
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"gosms/pkg/machine"
	"gosms/pkg/psg"
	"gosms/pkg/vdp"
)

// Frecuencia de muestreo del audio.
const sampleRate = 48000

// ---------- Audio ----------

// psgReader adapta el PSG (mono) al formato que exige Ebiten: PCM con signo de
// 16 bits, little-endian y DOS canales (estéreo). Si se le entrega una muestra
// por cada 2 bytes (mono), Ebiten la interpreta como (L,R): la reproducción se
// acelera al doble y todo suena una octava por encima y a destiempo.
type psgReader struct {
	psg  *psg.PSG
	mono []byte // memoria reutilizada para la mezcla mono del PSG
}

func (r *psgReader) Read(p []byte) (int, error) {
	n := len(p) / 4 // tramas estéreo = muestras mono (cada una va a L y a R)
	if n == 0 {
		return len(p), nil
	}
	need := n * 2
	if cap(r.mono) < need {
		r.mono = make([]byte, need)
	}
	m := r.mono[:need]
	r.psg.Render(m)
	for i := 0; i < n; i++ {
		lo, hi := m[i*2], m[i*2+1]
		p[i*4+0], p[i*4+1] = lo, hi // canal izquierdo
		p[i*4+2], p[i*4+3] = lo, hi // canal derecho
	}
	return len(p), nil
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

	// Rendimiento: sirve para comprobar que la emulación va a 60 Hz (si va más
	// lenta, la música también se reproduce más lenta, porque el motor de sonido
	// del juego avanza con la interrupción de frame).
	g.frames++
	if g.frames%30 == 0 {
		ebiten.SetWindowTitle(fmt.Sprintf("GoSMS - Sega Master System   %.0f fps  %.1f TPS",
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
	scale := math.Min(float64(sw)/float64(vdp.ScreenW), float64(sh)/float64(vdp.ScreenH))
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate((float64(sw)-float64(vdp.ScreenW)*scale)/2, (float64(sh)-float64(vdp.ScreenH)*scale)/2)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(g.img, op)
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

	audioCtx := audio.NewContext(sampleRate)
	player, err := audioCtx.NewPlayer(&psgReader{psg: m.PSG})
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
	}

	ebiten.SetWindowTitle("GoSMS - Sega Master System")
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
