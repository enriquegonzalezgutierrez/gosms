package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gosms/pkg/machine"
	"gosms/pkg/vdp"
)

type AuditResult struct {
	Name       string
	SizeKB     int
	HasHeader  bool
	VRAMActive bool
	CRAMActive bool
	MaxColors  int
	MaxScore   int
	FinalPC    uint16
	Status     string
	ErrorMsg   string
}

func evalScore(frame []byte) (int, int) {
	colors := make(map[uint16]int)
	samples := 0
	for i := 0; i < len(frame); i += 16 {
		r, g, b := frame[i]>>3, frame[i+1]>>3, frame[i+2]>>3
		c := uint16(r)<<10 | uint16(g)<<5 | uint16(b)
		colors[c]++
		samples++
	}
	if len(colors) <= 1 {
		return len(colors), 0
	}
	maxC := 0
	for _, cnt := range colors {
		if cnt > maxC {
			maxC = cnt
		}
	}
	if float64(maxC)/float64(samples) > 0.97 {
		return len(colors), 0
	}
	nonBg := samples - maxC
	score := len(colors)*100 + nonBg
	return len(colors), score
}

func auditRom(path string) AuditResult {
	res := AuditResult{
		Name: filepath.Base(path),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		res.Status = "ERROR"
		res.ErrorMsg = err.Error()
		return res
	}

	res.SizeKB = len(data) / 1024
	res.HasHeader = (len(data)%1024 == 512)

	defer func() {
		if r := recover(); r != nil {
			res.Status = "CRASH"
			res.ErrorMsg = fmt.Sprintf("%v", r)
		}
	}()

	m := machine.New(data)

	// Detección automática PAL (50Hz) para juegos europeos
	nameUpper := strings.ToUpper(path)
	if strings.Contains(nameUpper, "(E)") || strings.Contains(nameUpper, "(UE)") || strings.Contains(nameUpper, "EUROPE") {
		m.LinesPerFrame = 313
	}

	bestScore := 0
	maxCols := 0
	var bestFrame []byte

	maxF := 1200 // Margen generoso para intros largas
	for f := 0; f < maxF; f++ {
		// Pulsar botones 1 y 2 para saltar intros
		if f%40 >= 10 && f%40 <= 30 {
			m.CPU.Pad1 = 0xCF
		} else {
			m.CPU.Pad1 = 0xFF
		}
		m.RunFrame()

		cols, score := evalScore(m.VDP.Frame[:])
		if cols > maxCols {
			maxCols = cols
		}
		if score > bestScore {
			bestScore = score
			if bestFrame == nil {
				bestFrame = make([]byte, len(m.VDP.Frame))
			}
			copy(bestFrame, m.VDP.Frame[:])
		}
		for _, b := range m.VDP.CRAM {
			if b != 0 {
				res.CRAMActive = true
				break
			}
		}
	}

	res.MaxColors = maxCols
	res.MaxScore = bestScore
	res.FinalPC = m.CPU.PC

	for _, b := range m.VDP.VRAM {
		if b != 0 {
			res.VRAMActive = true
			break
		}
	}
	for _, b := range m.VDP.CRAM {
		if b != 0 {
			res.CRAMActive = true
			break
		}
	}
	if maxCols >= 4 && bestScore > 0 {
		res.CRAMActive = true
	}

	if (bestScore > 0 && res.VRAMActive && res.CRAMActive) || (maxCols >= 4 && res.VRAMActive) {
		res.Status = "OK"
	} else if !res.VRAMActive && !res.CRAMActive {
		res.Status = "COLGADO"
	} else {
		res.Status = "NEGRO"
	}

	if bestScore > 0 && len(bestFrame) > 0 {
		thumbDir := ".thumbnails"
		_ = os.MkdirAll(thumbDir, 0755)
		baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		thumbPath := filepath.Join(thumbDir, baseName+".png")

		shouldSave := false
		if f, err := os.Open(thumbPath); err != nil {
			shouldSave = true
		} else {
			img, err := png.Decode(f)
			f.Close()
			if err != nil {
				shouldSave = true
			} else {
				b := img.Bounds()
				rgba := image.NewRGBA(b)
				draw.Draw(rgba, b, img, image.Point{}, draw.Src)
				_, score := evalScore(rgba.Pix)
				if score <= 0 {
					shouldSave = true
				}
			}
		}

		if shouldSave {
			rgba := image.NewRGBA(image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH))
			copy(rgba.Pix, bestFrame)
			if outF, err := os.Create(thumbPath); err == nil {
				_ = png.Encode(outF, rgba)
				outF.Close()
			}
		}
	}

	return res
}

func main() {
	romsDir := "./roms"
	entries, err := os.ReadDir(romsDir)
	if err != nil {
		fmt.Printf("Error leyendo directorio %s: %v\n", romsDir, err)
		return
	}

	filter := ""
	if len(os.Args) > 1 {
		filter = strings.ToLower(os.Args[1])
	}
	var roms []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !e.IsDir() && (ext == ".sms" || ext == ".bin") {
			if filter == "" || strings.Contains(strings.ToLower(e.Name()), filter) {
				roms = append(roms, filepath.Join(romsDir, e.Name()))
			}
		}
	}

	fmt.Printf("\n🚀 INICIANDO AUDITORÍA DE %d JUEGOS...\n\n", len(roms))
	start := time.Now()

	okCount := 0
	blackCount := 0
	hungCount := 0
	crashCount := 0

	var failing []AuditResult

	for i, r := range roms {
		res := auditRom(r)
		fmt.Printf("[%3d/%3d] %-10s | %-40.40s | VRAM:%-5t | CRAM:%-5t | Cols:%-2d\n",
			i+1, len(roms), res.Status, res.Name, res.VRAMActive, res.CRAMActive, res.MaxColors)

		switch res.Status {
		case "OK":
			okCount++
		case "NEGRO":
			blackCount++
			failing = append(failing, res)
		case "COLGADO":
			hungCount++
			failing = append(failing, res)
		case "CRASH":
			crashCount++
			failing = append(failing, res)
		}
	}

	elapsed := time.Since(start)

	fmt.Println("\n" + strings.Repeat("=", 75))
	fmt.Printf("📊 RESUMEN DE COMPATIBILIDAD (%s):\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  ✅ FUNCIONANDO: %d (%.1f%%)\n", okCount, float64(okCount)/float64(len(roms))*100)
	fmt.Printf("  ⚫ PANTALLA NEGRA: %d\n", blackCount)
	fmt.Printf("  🛑 COLGADOS: %d\n", hungCount)
	fmt.Printf("  💥 CRASH: %d\n", crashCount)
	fmt.Println(strings.Repeat("=", 75))

	if len(failing) > 0 {
		fmt.Printf("\n📋 LISTADO DE JUEGOS CON FALLO (%d):\n", len(failing))
		for _, f := range failing {
			fmt.Printf("  - [%-7s] %-38s (PC: 0x%04X, %dKB, Header: %t) %s\n",
				f.Status, f.Name, f.FinalPC, f.SizeKB, f.HasHeader, f.ErrorMsg)
		}
	}
}
