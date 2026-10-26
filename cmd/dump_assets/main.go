package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"gosms/pkg/machine"
)

func decodeColor(c byte) color.RGBA {
	r := (c & 3) * 85
	g := ((c >> 2) & 3) * 85
	b := ((c >> 4) & 3) * 85
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func isPaletteValid(cram []byte) bool {
	nonZero := 0
	unique := make(map[byte]bool)
	for _, b := range cram {
		if b != 0 {
			nonZero++
		}
		unique[b] = true
	}
	return nonZero >= 3 && len(unique) >= 3
}

func decodeTile(tileBytes []byte, palBytes []byte, isSprite bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	pal := make([]color.RGBA, 16)
	for i := 0; i < 16; i++ {
		pal[i] = decodeColor(palBytes[i])
	}

	for y := 0; y < 8; y++ {
		b0 := tileBytes[y*4]
		b1 := tileBytes[y*4+1]
		b2 := tileBytes[y*4+2]
		b3 := tileBytes[y*4+3]
		for x := 0; x < 8; x++ {
			bit := 7 - x
			cIdx := ((b0 >> bit) & 1) | (((b1 >> bit) & 1) << 1) | (((b2 >> bit) & 1) << 2) | (((b3 >> bit) & 1) << 3)
			col := pal[cIdx&0x0F]
			if isSprite && cIdx == 0 {
				col.A = 0 // Solo los sprites tienen canal alfa transparente
			} else {
				col.A = 255 // Los fondos son siempre 100% opacos
			}
			img.Set(x, y, col)
		}
	}
	return img
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
		fmt.Println("Uso: dump_assets <rom.sms>")
		return
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
	os.MkdirAll(outDir, 0755)
	os.MkdirAll(hdDir, 0755)

	fmt.Printf("🚀 Ejecutando volcado offline de alta fidelidad para '%s'...\n", baseName)
	m := machine.New(data)

	uniqueTiles := make(map[string]*image.RGBA)
	var manifest []machine.TileManifest
	tileCount := 0

	addTile := func(tileData []byte, cram []byte, isSprite bool) {
		if isBlankTile(tileData) {
			return
		}
		h := machine.TileHash(tileData, cram, isSprite)
		if _, exists := uniqueTiles[h]; !exists {
			uniqueTiles[h] = decodeTile(tileData, cram, isSprite)
			manifest = append(manifest, machine.TileManifest{
				Hash:     h,
				Index:    tileCount,
				IsSprite: isSprite,
			})
			tileCount++
		}
	}

	for f := 0; f < 6500; f++ {
		pad := byte(0xFF)
		if f < 300 {
			pad = 0xFF // Logo Sega
		} else if f < 550 {
			if (f/12)%2 == 0 {
				pad &= ^byte(0x10) // Start en título
			}
		} else if f < 750 {
			pad = 0xFF // Mapa de la isla Green Hill
		} else {
			pad &= ^byte(0x08) // Correr derecha
			if (f % 120) < 20 {
				pad &= ^byte(0x10) // Saltar obstáculos
			}
		}
		m.CPU.Pad1 = pad
		m.RunFrame()

		if f%2 == 0 {
			v := m.VDP
			bgCram0 := v.CRAM[0:16]
			bgCram1 := v.CRAM[16:32]
			sprCram := v.CRAM[16:32]

			// Capturar tiles visibles del fondo
			if isPaletteValid(bgCram0) || isPaletteValid(bgCram1) {
				nameBase := int(v.Reg[2]&0x0E) << 10
				for row := 0; row < 28; row++ {
					for col := 0; col < 32; col++ {
						off := (nameBase + row*64 + col*2) & 0x3FFF
						entry := uint16(v.VRAM[(off+1)&0x3FFF])<<8 | uint16(v.VRAM[off])
						tile := int(entry & 0x1FF)
						pal := entry&0x800 != 0
						pat := (tile * 32) & 0x3FFF
						tileData := v.VRAM[pat : pat+32]

						cram := bgCram0
						if pal {
							cram = bgCram1
						}
						if isPaletteValid(cram) {
							addTile(tileData, cram, false)
						}
					}
				}
			}

			// Capturar sprites visibles en pantalla
			if isPaletteValid(sprCram) {
				satBase := int(v.Reg[5]&0x7E) << 7
				patBase := int(v.Reg[6]&0x04) << 11
				tall := v.Reg[1]&0x02 != 0

				for i := 0; i < 64; i++ {
					y := int(v.VRAM[(satBase+i)&0x3FFF])
					if y == 0xD0 {
						break
					}
					tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
					parts := 1
					if tall {
						parts = 2
						tile &^= 1
					}
					for p := 0; p < parts; p++ {
						pat := (patBase + (tile+p)*32) & 0x3FFF
						tileData := v.VRAM[pat : pat+32]
						addTile(tileData, sprCram, true)
					}
				}
			}
		}
	}

	fmt.Printf("📦 Generando Spritesheet Anti-Bleed (%d texturas únicas)...\n", len(uniqueTiles))

	cols := 32
	rows := (len(uniqueTiles) + cols - 1) / cols
	if rows == 0 {
		rows = 1
	}

	cellSize := 12
	padding := 2
	sheet := image.NewRGBA(image.Rect(0, 0, cols*cellSize, rows*cellSize))

	for _, mItem := range manifest {
		tImg := uniqueTiles[mItem.Hash]
		gx := (mItem.Index % cols) * cellSize
		gy := (mItem.Index / cols) * cellSize

		// 1. Copiar el tile central de 8x8
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				sheet.Set(gx+padding+x, gy+padding+y, tImg.At(x, y))
			}
		}

		// 2. Texture Gutter Clamping: clonar los píxeles de los bordes hacia el margen
		// para evitar cualquier tipo de sangrado o cuadrícula en el reescalado IA
		for y := 0; y < 8; y++ {
			colL := tImg.At(0, y)
			colR := tImg.At(7, y)
			sheet.Set(gx+0, gy+padding+y, colL)
			sheet.Set(gx+1, gy+padding+y, colL)
			sheet.Set(gx+10, gy+padding+y, colR)
			sheet.Set(gx+11, gy+padding+y, colR)
		}
		for x := 0; x < 12; x++ {
			cx := x - padding
			if cx < 0 {
				cx = 0
			}
			if cx > 7 {
				cx = 7
			}
			colT := tImg.At(cx, 0)
			colB := tImg.At(cx, 7)
			sheet.Set(gx+x, gy+0, colT)
			sheet.Set(gx+x, gy+1, colT)
			sheet.Set(gx+x, gy+10, colB)
			sheet.Set(gx+x, gy+11, colB)
		}
	}

	sf, err := os.Create(filepath.Join(outDir, "spritesheet.png"))
	if err == nil {
		png.Encode(sf, sheet)
		sf.Close()
	}

	saveJSON(filepath.Join(outDir, "manifest.json"), manifest)
	saveJSON(filepath.Join(hdDir, "manifest.json"), manifest)

	fmt.Printf("✅ VOLCADO COMPLETADO CON ÉXITO (%d texturas)\n", len(manifest))
}

func saveJSON(path string, data interface{}) {
	f, _ := os.Create(path)
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}
