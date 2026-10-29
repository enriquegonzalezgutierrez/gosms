// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command preview renders a single emulator frame to a PNG using the same
// post-processing pipeline as the desktop emulator. It is useful for comparing
// the original frame, the modern preset and the HD remaster without opening the
// interactive launcher.
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
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"gosms/pkg/machine"
	"gosms/pkg/vdp"
	"gosms/pkg/video"
)

type previewGame struct {
	src      *ebiten.Image
	srcRect  image.Rectangle
	dstRect  image.Rectangle
	preset   video.Preset
	opts     video.Options
	out      string
	width    int
	height   int
	pipeline *video.Pipeline
	done     bool
}

func (p *previewGame) Update() error {
	if p.done {
		return ebiten.Termination
	}
	return nil
}

func (p *previewGame) Draw(screen *ebiten.Image) {
	if p.done {
		return
	}
	screen.Fill(color.Black)
	if err := p.pipeline.Draw(screen, p.src, p.srcRect, p.dstRect, p.opts); err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, p.width*p.height*4)
	screen.ReadPixels(buf)
	rgba := &image.RGBA{Pix: buf, Stride: p.width * 4, Rect: image.Rect(0, 0, p.width, p.height)}
	f, err := os.Create(p.out)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, rgba); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	p.done = true
}

func (p *previewGame) Layout(_, _ int) (int, int) { return p.width, p.height }

func inputForFrame(frame int, bot string) byte {
	pad := byte(0xFF)
	if frame >= 300 && frame < 550 && (frame/12)%2 == 0 {
		pad &^= 0x10
	}
	if frame < 750 {
		return pad
	}
	if bot == "alex-kidd" {
		relative := frame - 750
		switch (relative / 240) % 6 {
		case 0, 1, 5:
			pad &^= 0x08
		case 2:
			pad &^= 0x09
		case 3:
			pad &^= 0x0A
		case 4:
			pad &^= 0x04
		}
		if relative%36 < 20 {
			pad &^= 0x30
		}
		return pad
	}
	pad &^= 0x08
	if (frame-750)%120 < 20 {
		pad &^= 0x10
	}
	return pad
}

func decodeColorVDP(c byte) color.RGBA {
	r := (c & 3) * 85
	g := ((c >> 2) & 3) * 85
	b := ((c >> 4) & 3) * 85
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func parsePreset(name string) (video.Preset, bool) {
	switch name {
	case "pixel", "pixel-perfect":
		return video.PresetPixelPerfect, false
	case "smooth":
		return video.PresetSmooth, false
	case "modern", "remaster":
		return video.PresetModern, false
	case "modern+", "modern-plus", "modernplus":
		return video.PresetModernPlus, false
	case "modern-parallax", "modernparallax", "parallax":
		return video.PresetModernPlus, false
	case "crt", "scanlines":
		return video.PresetCRT, name == "scanlines"
	case "mono", "monochrome":
		return video.PresetMonochrome, false
	default:
		return 0, false
	}
}

type padPress struct {
	start int
	pad   byte
	len   int
}

func parsePadScript(s string) []padPress {
	var out []padPress
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, ":")
		if len(fields) < 2 {
			continue
		}
		var start, length int
		var pad uint
		fmt.Sscanf(fields[0], "%d", &start)
		fmt.Sscanf(fields[1], "%x", &pad)
		length = 8
		if len(fields) > 2 {
			fmt.Sscanf(fields[2], "%d", &length)
		}
		out = append(out, padPress{start: start, pad: byte(pad), len: length})
	}
	return out
}

type ramPoke struct {
	frame int
	addr  uint16
	val   byte
}

func parsePokes(s string) []ramPoke {
	var out []ramPoke
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, ":") || !strings.Contains(part, "=") {
			continue
		}
		fields := strings.SplitN(part, ":", 2)
		kv := strings.SplitN(fields[1], "=", 2)
		var frame int
		var addr, val uint
		fmt.Sscanf(fields[0], "%d", &frame)
		fmt.Sscanf(kv[0], "%x", &addr)
		fmt.Sscanf(kv[1], "%x", &val)
		out = append(out, ramPoke{frame: frame, addr: uint16(addr), val: byte(val)})
	}
	return out
}

func main() {
	frames := flag.Int("frames", 900, "número de frames a emular antes de la captura")
	bot := flag.String("bot", "alex-kidd", "perfil de controles: default o alex-kidd")
	width := flag.Int("width", 1920, "ancho de la captura")
	height := flag.Int("height", 1080, "alto de la captura")
	out := flag.String("out", "preview.png", "fichero PNG de salida")
	hd := flag.Bool("hd", false, "usar el pack HD de assets/<ROM>/hd")
	crop := flag.Bool("crop", true, "recortar los 8 píxeles de overscan izquierdo")
	filter := flag.String("filter", "modern", "pixel, smooth, modern, crt, scanlines o mono")
	noSky := flag.Bool("nosky", false, "desactiva el degradado moderno del cielo")
	parallax := flag.Bool("parallax", false, "activa el fondo lejano procedural (MODERN+ PARALLAX)")
	debug := flag.Bool("debug", false, "imprime estado del VDP tras emular")
	scrollX := flag.Float64("scrollx", -1, "fuerza el scroll X normalizado (pruebas de parallax)")
	skyTime := flag.Float64("time", 45, "segundos del ciclo día/noche para la captura")
	dayCycle := flag.Float64("cycle", 180, "duración del ciclo día/noche en segundos (0 lo desactiva)")
	pause := flag.Int("pause", -1, "pulsa PAUSE/NMI en este frame (util para capturar el mapa)")
	script := flag.String("script", "", "guion de entradas: frame:padhex:frames,...")
	poke := flag.String("poke", "", "pokes de RAM: frame:addr=val,...")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Uso: preview [opciones] <rom.sms>")
		os.Exit(2)
	}
	if *width < 1 || *height < 1 {
		fmt.Fprintln(os.Stderr, "width y height deben ser positivos")
		os.Exit(2)
	}
	preset, onlyScanlines := parsePreset(*filter)
	romPath := flag.Arg(0)
	rom, err := os.ReadFile(romPath)
	if err != nil {
		log.Fatal(err)
	}
	m := machine.New(rom)
	presses := parsePadScript(*script)
	pokes := parsePokes(*poke)
	for i := 0; i < *frames; i++ {
		if i == *pause {
			m.CPU.NMI = true
		}
		for _, p := range pokes {
			if i >= p.frame {
				m.CPU.RAM[(p.addr-0xC000)&0x1FFF] = p.val
			}
		}
		pad := inputForFrame(i, *bot)
		if len(presses) > 0 {
			pad = 0xFF
			for _, p := range presses {
				if i >= p.start && i < p.start+p.len {
					pad &= p.pad
				}
			}
		}
		m.CPU.Pad1 = pad
		m.RunFrame()
	}

	if *debug {
		fmt.Printf("frame=%d scrollX=%d scrollY=%d line0=%d\n", *frames, m.VDP.Reg[8], m.VDP.Reg[9], m.VDP.LineScrollX[0])
	}

	var src *ebiten.Image
	var srcRect image.Rectangle
	if *hd {
		machine.InitHD(romPath)
		if machine.HD == nil || !machine.HD.Active {
			log.Fatalf("no hay pack HD disponible para %s", romPath)
		}
		minX := 0
		if *crop {
			minX = 8
		}
		w := (vdp.ScreenW - minX) * machine.HD.Scale
		h := vdp.ScreenH * machine.HD.Scale
		layer := ebiten.NewImage(w, h)
		machine.HD.DrawLayer(m.VDP, layer, nil, float64(machine.HD.Scale), 0, 0, *crop)
		src = layer
		srcRect = image.Rect(0, 0, w, h)
	} else {
		frame := ebiten.NewImage(vdp.ScreenW, vdp.ScreenH)
		frame.WritePixels(m.VDP.Frame[:])
		src = frame
		if *crop {
			srcRect = image.Rect(8, 0, vdp.ScreenW, vdp.ScreenH)
		} else {
			srcRect = image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH)
		}
	}

	scale := math.Min(float64(*width)/float64(srcRect.Dx()), float64(*height)/float64(srcRect.Dy()))
	dstW := int(math.Round(float64(srcRect.Dx()) * scale))
	dstH := int(math.Round(float64(srcRect.Dy()) * scale))
	dstRect := image.Rect((*width-dstW)/2, (*height-dstH)/2, (*width-dstW)/2+dstW, (*height-dstH)/2+dstH)

	opts := video.DefaultOptions(preset, *hd)
	opts.HeightLit = 0
	opts.Relief = 0
	if !*noSky && (preset == video.PresetModern || preset == video.PresetModernPlus ||
		preset == video.PresetCRT) {
		raw := decodeColorVDP(m.VDP.CRAM[16+int(m.VDP.Reg[7]&0x0F)])
		c := raw
		if *hd && machine.HD != nil && machine.HD.Active {
			c = machine.HD.Backdrop(m.VDP)
		}
		if !machine.IsSkyColor(c) && !machine.IsSkyColor(raw) {
			for i := 0; i < len(m.VDP.CRAM); i++ {
				cc := decodeColorVDP(m.VDP.CRAM[i])
				if machine.IsSkyColor(cc) {
					raw = cc
					if *hd && machine.HD != nil && machine.HD.Active {
						c = machine.HD.GradeColor(cc)
					} else {
						c = cc
					}
					break
				}
			}
		}
		if machine.IsSkyColor(c) || machine.IsSkyColor(raw) {
			opts.Sky = true
			opts.SkyColor = [3]float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255}
			opts.SkyColor2 = [3]float32{float32(raw.R) / 255, float32(raw.G) / 255, float32(raw.B) / 255}
		}
	}
	if *parallax || *filter == "modern-parallax" || *filter == "modernparallax" || *filter == "parallax" {
		opts.Parallax = true
		sx := int(m.VDP.Reg[8])
		if m.VDP.LineScrollValid {
			sx = int(m.VDP.LineScrollX[0])
		}
		opts.ScrollX = float32(sx) / float32(vdp.ScreenW)
		opts.ScrollY = float32(m.VDP.Reg[9]) / float32(vdp.ScreenH)
		opts.Time = float32(*skyTime)
		opts.CycleSeconds = float32(*dayCycle)
		if *scrollX >= 0 {
			opts.ScrollX = float32(*scrollX)
		}
	}
	if onlyScanlines {
		opts.Bloom = 0
		opts.Sharpness = 0
		opts.Saturation = 1
		opts.Contrast = 1
		opts.Gamma = 1
		opts.Brightness = 1
		opts.Mask = 0
		opts.Vignette = 0
		opts.Curvature = 0
		opts.Scanline = 0.22
	}

	p := &previewGame{
		src: src, srcRect: srcRect, dstRect: dstRect,
		preset: preset, opts: opts, out: *out,
		width: *width, height: *height, pipeline: video.New(),
	}
	ebiten.SetWindowSize(p.width, p.height)
	if err := ebiten.RunGame(p); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("captura escrita en %s (%dx%d, filtro %s, HD=%v)\n", p.out, p.width, p.height, *filter, *hd)
}
