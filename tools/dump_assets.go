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
	tilesDir := filepath.Join(outDir, "tiles")
	_ = os.MkdirAll(tilesDir, 0755)

	fmt.Printf("🚀 Ejecutando '%s' en modo fantasma para cazar texturas...\n", baseName)
	m := machine.New(data)

	uniqueTiles := make(map[string]*image.RGBA)
	var manifest []TileManifest
	tileCount := 0

	// Simular 3000 frames (50 segundos de juego) a máxima velocidad
	for f := 0; f < 3000; f++ {
		// IA Jugadora: Pulsar START (Botón 1) y DERECHA alternativamente para pasar menús y empezar el nivel
		pad := byte(0xFF)
		if f%120 < 10 {
			pad &= ^byte(0x10) // Botón 1 (Pulsar Start/Saltar)
		} else if f > 600 {
			pad &= ^byte(0x08) // Mantener Derecha
		}
		m.CPU.Pad1 = pad
		m.RunFrame()

		// Cada 60 frames (1 segundo), escanear la VRAM descomprimida
		if f%60 == 0 {
			bgPalette := make([]color.RGBA, 16)
			sprPalette := make([]color.RGBA, 16)
			for i := 0; i < 16; i++ {
				bgPalette[i] = decodeColor(m.VDP.CRAM[i])
				sprPalette[i] = decodeColor(m.VDP.CRAM[16+i])
			}

			vram := m.VDP.VRAM[:]
			for i := 0; i < 0x3800; i += 32 {
				chunk := vram[i : i+32]
				if isBlankTile(chunk) {
					continue
				}
				for pIdx, pal := range [][]color.RGBA{bgPalette, sprPalette} {
					h := tileHash(chunk, pIdx)
					if _, exists := uniqueTiles[h]; !exists {
						uniqueTiles[h] = decodeTile(chunk, pal)
						manifest = append(manifest, TileManifest{Hash: h, Index: tileCount, PaletteIdx: pIdx})
						tileCount++
					}
				}
			}
		}
	}

	fmt.Printf("📦 Guardando %d tiles individuales reales...\n", len(uniqueTiles))
	for h, img := range uniqueTiles {
		f, _ := os.Create(filepath.Join(tilesDir, fmt.Sprintf("tile_%s.png", h)))
		png.Encode(f, img)
		f.Close()
	}

	// Crear Spritesheet Global
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

	mfPath := filepath.Join(outDir, "manifest.json")
	mf, _ := os.Create(mfPath)
	enc := json.NewEncoder(mf)
	enc.SetIndent("", "  ")
	enc.Encode(manifest)
	mf.Close()

	fmt.Printf("✅ EXTRACCIÓN DINÁMICA COMPLETADA\n")
	fmt.Printf("   Mosaico listo para la IA en: %s\n", sheetPath)
}
