package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"alexkidd/pkg/machine"
)

type TileManifest struct {
	Hash       string `json:"hash"`
	Index      int    `json:"index"`
	PaletteIdx int    `json:"palette"`
}

func decodeColor(c byte) color.RGBA {
	r := (c & 3) * 85
	g := ((c >> 2) & 3) * 85
	b := ((c >> 4) & 3) * 85
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func isPaletteValid(pal []color.RGBA) bool {
	nonBlack := 0
	for _, c := range pal {
		if c.R > 0 || c.G > 0 || c.B > 0 {
			nonBlack++
		}
	}
	return nonBlack >= 4
}

func decodeTile(tileBytes []byte, palette []color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		b0, b1, b2, b3 := tileBytes[y*4], tileBytes[y*4+1], tileBytes[y*4+2], tileBytes[y*4+3]
		for x := 0; x < 8; x++ {
			bit := 7 - x
			colorIdx := ((b0>>bit)&1) | (((b1>>bit)&1)<<1) | (((b2>>bit)&1)<<2) | (((b3>>bit)&1)<<3)
			col := palette[colorIdx&0x0F]
			if colorIdx == 0 {
				col.A = 0
			}
			img.Set(x, y, col)
		}
	}
	return img
}

func tileHash(data []byte, palIdx int) string {
	h := fnv.New64a()
	h.Write(data)
	h.Write([]byte{byte(palIdx)})
	return fmt.Sprintf("%016x", h.Sum64())
}

func isBlankTile(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Error: Falta la ruta de la ROM")
		os.Exit(1)
	}
	romPath := os.Args[1]
	data, err := os.ReadFile(romPath)
	if err != nil {
		fmt.Printf("Error leyendo ROM: %v\n", err)
		return
	}

	baseName := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	outDir := filepath.Join(".", "assets", baseName, "original")
	hdDir := filepath.Join(".", "assets", baseName, "hd")

	_ = os.MkdirAll(outDir, 0755)
	_ = os.MkdirAll(hdDir, 0755)

	fmt.Printf("🚀 Extrayendo texturas de '%s'...\n", baseName)
	m := machine.New(data)

	uniqueTiles := make(map[string]*image.RGBA)
	var manifest []TileManifest
	tileCount := 0

	for f := 0; f < 5500; f++ {
		pad := byte(0xFF)
		if f < 350 {
			pad = 0xFF // Portada
		} else if f < 550 {
			if (f/15)%2 == 0 { pad &= ^byte(0x10) } // Start
		} else if f < 750 {
			pad = 0xFF // Mapa de la isla
		} else {
			pad &= ^byte(0x08) // Green Hill
			if (f%140) < 25 { pad &= ^byte(0x10) }
		}

		m.CPU.Pad1 = pad
		m.RunFrame()

		esVentanaValida := (f >= 300 && f <= 380) || (f >= 600)
		if esVentanaValida && f%12 == 0 {
			bgPalette := make([]color.RGBA, 16)
			sprPalette := make([]color.RGBA, 16)
			for i := 0; i < 16; i++ {
				bgPalette[i] = decodeColor(m.VDP.CRAM[i])
				sprPalette[i] = decodeColor(m.VDP.CRAM[16+i])
			}

			if !isPaletteValid(bgPalette) && !isPaletteValid(sprPalette) {
				continue
			}

			vram := m.VDP.VRAM[:]
			for i := 0; i < 0x4000; i += 32 {
				chunk := vram[i : i+32]
				if isBlankTile(chunk) {
					continue
				}

				palettes := []struct {
					pal  []color.RGBA
					pIdx int
				}{
					{bgPalette, 0},
					{sprPalette, 1},
				}

				for _, p := range palettes {
					if !isPaletteValid(p.pal) { continue }
					h := tileHash(chunk, p.pIdx)
					if _, exists := uniqueTiles[h]; !exists {
						uniqueTiles[h] = decodeTile(chunk, p.pal)
						manifest = append(manifest, TileManifest{Hash: h, Index: tileCount, PaletteIdx: p.pIdx})
						tileCount++
					}
				}
			}
		}
	}

	fmt.Printf("📦 Texturas únicas: %d\n", len(uniqueTiles))

	cols := 32
	rows := (len(uniqueTiles) + cols - 1) / cols
	if rows == 0 { rows = 1 }

	sheet := image.NewRGBA(image.Rect(0, 0, cols*8, rows*8))
	idx := 0
	for _, mItem := range manifest {
		tImg := uniqueTiles[mItem.Hash]
		gx := (idx % cols) * 8
		gy := (idx / cols) * 8
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				sheet.Set(gx+x, gy+y, tImg.At(x, y))
			}
		}
		idx++
	}

	sheetPath := filepath.Join(outDir, "spritesheet.png")
	sf, _ := os.Create(sheetPath)
	png.Encode(sf, sheet)
	sf.Close()

	saveJSON(filepath.Join(outDir, "manifest.json"), manifest)
	saveJSON(filepath.Join(hdDir, "manifest.json"), manifest)

	fmt.Printf("✅ Spritesheet base perfecto generado en: %s\n", sheetPath)
}

func saveJSON(path string, data interface{}) {
	f, err := os.Create(path)
	if err != nil { return }
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}
