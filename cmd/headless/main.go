package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"gosms/pkg/cpu"
	"gosms/pkg/machine"
	"gosms/pkg/vdp"
)

// writeWAV escribe PCM con signo de 16 bits little-endian como WAV.
func writeWAV(path string, pcm []byte, rate int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dataLen := len(pcm)
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+dataLen))
	copy(hdr[8:], "WAVE")
	copy(hdr[12:], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:], 1) // mono
	binary.LittleEndian.PutUint32(hdr[24:], uint32(rate))
	binary.LittleEndian.PutUint32(hdr[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(hdr[32:], 2)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(dataLen))
	if _, err := f.Write(hdr); err != nil {
		return err
	}
	_, err = f.Write(pcm)
	return err
}

func main() {
	frames := flag.Int("frames", 120, "número de frames a ejecutar")
	out := flag.String("out", "frame.png", "fichero PNG de salida")
	dump := flag.String("dump", "", "volcado de VRAM addr:len en hex (p.ej. 3f00:40)")
	regtrace := flag.Bool("regtrace", false, "traza las escrituras a registros del VDP")
	nosprites := flag.Bool("nosprites", false, "desactiva el dibujado de sprites (depuración)")
	start := flag.Int("start", -1, "frame en el que se pulsa START (botón 1) durante 8 frames")
	nt := flag.Bool("nt", false, "volcar la name table (28x32 índices de tile)")
	wdump := flag.String("wdump", "", "rango addr:len para contar escrituras a VRAM en el último frame")
	wlog := flag.String("wlog", "", "rango addr:len para registrar la secuencia de escrituras a VRAM")
	watch := flag.String("watch", "", "dirección de RAM a vigilar (escrituras), en hex")
	wav := flag.String("wav", "", "fichero WAV donde volcar el audio generado")
	bench := flag.Bool("bench", false, "mide el rendimiento y no escribe PNG")
	benchWarmup := flag.Int("bench-warmup", 60, "frames descartados antes de medir con -bench")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Uso: headless [-frames N] [-out frame.png] <juego.sms>")
		os.Exit(1)
	}

	rom, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error al cargar la ROM: %v\n", err)
		os.Exit(1)
	}

	m := machine.New(rom)
	frameNo := 0
	if *watch != "" {
		var wa int
		if _, err := fmt.Sscanf(*watch, "%x", &wa); err == nil {
			cpu.WatchAddr = uint16(wa)
		}
		cpu.MemWatch = func(addr uint16, val byte, c *cpu.CPU) {
			fmt.Printf("W %04X <- %02X @frame=%d pc=%04X A=%02X BC=%04X DE=%04X HL=%04X IX=%04X SP=%04X banks=%v\n",
				addr, val, frameNo, c.PC, c.A, uint16(c.B)<<8|uint16(c.C), uint16(c.D)<<8|uint16(c.E), uint16(c.H)<<8|uint16(c.L), c.IX, c.SP, c.Bank)
		}
	}
	vdp.ContextHook = func() (uint16, byte, uint16) {
		return m.CPU.PC, m.CPU.A, uint16(m.CPU.H)<<8 | uint16(m.CPU.L)
	}
	vdp.SetWriteDebug(*wdump != "")
	if *wlog != "" {
		var addr, length int
		if _, err := fmt.Sscanf(*wlog, "%x:%d", &addr, &length); err == nil {
			vdp.SetWriteLog(uint16(addr&0x3FFF), uint16((addr+length)&0x3FFF))
		}
	}
	vdp.SetDebugRegs(*regtrace)
	vdp.SetSpritesEnabled(!*nosprites)
	var samples []byte
	audioAcc := 0.0
	const sr = 48000
	samplesPerFrame := float64(sr) * float64(vdp.CyclesPerLine*vdp.TotalLines) / 3579545
	m.CPU.PSG.SetSampleRate(sr)
	var benchStart time.Time
	var benchCycles, benchInstr uint64
	for i := 0; i < *frames; i++ {
		if *bench && i == *benchWarmup {
			benchStart = time.Now()
			benchCycles = m.CPU.Cycles
			benchInstr = m.CPU.Instr
		}
		frameNo = i
		m.CPU.Pad1 = 0xFF
		if *start >= 0 && i >= *start && i < *start+8 {
			m.CPU.Pad1 = 0xEF // botón 1 (START) pulsado
		}
		m.RunFrame()
		if *wav != "" {
			audioAcc += samplesPerFrame
			n := int(audioAcc)
			audioAcc -= float64(n)
			b := make([]byte, n*2)
			m.PSG.Render(b)
			samples = append(samples, b...)
		}
	}
	vdp.SetWriteDebug(false)
	vdp.SetDebugRegs(false)

	if *wlog != "" {
		fmt.Printf("--- wlog (%d escrituras) ---\n", len(vdp.DebugLog))
		for i, r := range vdp.DebugLog {
			if i%16 == 0 {
				fmt.Printf("\n%04d:", i)
			}
			fmt.Printf(" %04X=%02X@%04X:A%02X,HL%04X", r.Addr, r.Val, r.PC, r.A, r.HL)
		}
		fmt.Println()
	}

	fmt.Printf("frames=%d pc=$%04X cyc=%d instr=%d status=$%02X regs=%v\n",
		*frames, m.CPU.PC, m.CPU.Cycles, m.CPU.Instr, m.VDP.Status, m.VDP.Reg[:11])

	if *bench {
		timed := *frames - *benchWarmup
		elapsed := time.Since(benchStart)
		if timed <= 0 || elapsed <= 0 {
			fmt.Fprintf(os.Stderr, "bench: el warmup (%d) debe ser menor que frames (%d)\n", *benchWarmup, *frames)
		} else {
			secs := elapsed.Seconds()
			fmt.Printf("bench frames=%d warmup=%d elapsed=%.3fs ms_per_frame=%.3f fps=%.1f cycles_per_frame=%.0f instr_per_frame=%.0f\n",
				timed, *benchWarmup, secs,
				elapsed.Seconds()*1000/float64(timed),
				float64(timed)/secs,
				float64(m.CPU.Cycles-benchCycles)/float64(timed),
				float64(m.CPU.Instr-benchInstr)/float64(timed))
		}
	}

	if *out == "" || *bench {
		return
	}

	img := image.NewRGBA(image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH))
	copy(img.Pix, m.VDP.Frame[:])

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error al crear la imagen: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintf(os.Stderr, "Error al codificar PNG: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("imagen escrita en %s\n", *out)

	if *wav != "" {
		var peak int
		for i := 0; i+1 < len(samples); i += 2 {
			s := int(int16(uint16(samples[i]) | uint16(samples[i+1])<<8))
			if s < 0 {
				s = -s
			}
			if s > peak {
				peak = s
			}
		}
		if err := writeWAV(*wav, samples, sr); err != nil {
			fmt.Fprintf(os.Stderr, "Error al escribir WAV: %v\n", err)
		} else {
			fmt.Printf("wav escrito en %s (%d muestras, pico=%d)\n", *wav, len(samples)/2, peak)
		}
	}

	if *wdump != "" {
		var addr, length int
		if _, err := fmt.Sscanf(*wdump, "%x:%d", &addr, &length); err == nil {
			for i := 0; i < length; i++ {
				a := (addr + i) & 0x3FFF
				c := vdp.DebugWrites[a]
				if c != 0 {
					fmt.Printf("%04X: %d\n", a, c)
				}
			}
		}
	}

	if *nt {
		base := int(m.VDP.Reg[2]&0x0E) << 10
		for row := 0; row < 28; row++ {
			fmt.Printf("%2d: ", row)
			for col := 0; col < 32; col++ {
				off := (base + row*64 + col*2) & 0x3FFF
				e := uint16(m.VDP.VRAM[(off+1)&0x3FFF])<<8 | uint16(m.VDP.VRAM[off])
				fmt.Printf("%03X ", e&0x1FF)
			}
			fmt.Println()
		}
	}

	if *dump != "" {
		var addr, length int
		if _, err := fmt.Sscanf(*dump, "%x:%d", &addr, &length); err == nil {
			for i := 0; i < length; i++ {
				if i%16 == 0 {
					fmt.Printf("\n%04X:", addr+i)
				}
				fmt.Printf(" %02X", m.VDP.VRAM[(addr+i)&0x3FFF])
			}
			fmt.Println()
		}
	}
}
