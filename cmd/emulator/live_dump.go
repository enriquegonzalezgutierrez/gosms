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
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"gosms/pkg/machine"
	"gosms/pkg/vdp"
)

type LiveDumper struct {
	Active      bool
	UniqueTiles map[string]*image.RGBA
	Manifest    []machine.TileManifest
	Document    *machine.TileManifestDocument
	BaseName    string
	OutDir      string
	HdDir       string
	ShotDir     string
	NewTiles    int
}

func newLiveDumper(romPath string) *LiveDumper {
	base := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	outDir := filepath.Join(".", "assets", base, "original")
	hdDir := filepath.Join(".", "assets", base, "hd")
	shotDir := filepath.Join(".", "assets", base, "screenshots")
	os.MkdirAll(outDir, 0755)
	os.MkdirAll(hdDir, 0755)
	os.MkdirAll(shotDir, 0755)

	ld := &LiveDumper{
		Active:      false,
		UniqueTiles: make(map[string]*image.RGBA),
		BaseName:    base,
		OutDir:      outDir,
		HdDir:       hdDir,
		ShotDir:     shotDir,
	}
	ld.loadExisting()
	return ld
}

func (ld *LiveDumper) loadExisting() {
	mfPath := filepath.Join(ld.OutDir, "manifest.json")
	sheetPath := filepath.Join(ld.OutDir, "spritesheet.png")
	if _, err := os.Stat(mfPath); os.IsNotExist(err) {
		return
	}
	if _, err := os.Stat(sheetPath); os.IsNotExist(err) {
		return
	}

	mfData, err := os.ReadFile(mfPath)
	if err != nil {
		return
	}
	var mf []machine.TileManifest
	if len(mfData) > 0 && mfData[0] == '[' {
		if err := json.Unmarshal(mfData, &mf); err != nil {
			return
		}
	} else {
		var document machine.TileManifestDocument
		if err := json.Unmarshal(mfData, &document); err != nil {
			return
		}
		ld.Document = &document
		mf = document.Tiles
	}

	f, err := os.Open(sheetPath)
	if err != nil {
		return
	}
	defer f.Close()
	sheetImg, err := png.Decode(f)
	if err != nil {
		return
	}

	cellSize := 12
	padding := 2
	cols := 32

	for _, item := range mf {
		gx := (item.Index%cols)*cellSize + padding
		gy := (item.Index/cols)*cellSize + padding

		tImg := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				tImg.Set(x, y, sheetImg.At(gx+x, gy+y))
			}
		}
		ld.UniqueTiles[item.Hash] = tImg
		ld.Manifest = append(ld.Manifest, item)
	}
}

func isPaletteCRAMValid(cram []byte) bool {
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

func decodeTileLive(tileBytes []byte, palBytes []byte, isSprite bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	pal := make([]color.RGBA, 16)
	for i := 0; i < 16; i++ {
		pal[i] = decodeColorVDP(palBytes[i])
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
				col.A = 0
			} else {
				col.A = 255
			}
			img.Set(x, y, col)
		}
	}
	return img
}

func isTileBlank(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func (ld *LiveDumper) CaptureFrame(v *vdp.VDP) {
	if !ld.Active || v == nil {
		return
	}

	bgCram0 := v.CRAM[0:16]
	bgCram1 := v.CRAM[16:32]
	sprCram := v.CRAM[16:32]

	addTile := func(tileData []byte, cram []byte, isSprite bool) {
		if isTileBlank(tileData) {
			return
		}
		h := machine.TileHash(tileData, cram, isSprite)
		if _, exists := ld.UniqueTiles[h]; !exists {
			ld.UniqueTiles[h] = decodeTileLive(tileData, cram, isSprite)
			ld.Manifest = append(ld.Manifest, machine.TileManifest{
				Hash:     h,
				Index:    len(ld.Manifest),
				IsSprite: isSprite,
			})
			ld.NewTiles++
		}
	}

	// 1. Fondos visibles en pantalla
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
			if isPaletteCRAMValid(cram) {
				addTile(tileData, cram, false)
			}
		}
	}

	// 2. Sprites visibles en pantalla
	if isPaletteCRAMValid(sprCram) {
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

func (ld *LiveDumper) Save() int {
	total := len(ld.Manifest)
	if total == 0 {
		return 0
	}

	cols := 32
	rows := (total + cols - 1) / cols
	if rows == 0 {
		rows = 1
	}

	cellSize := 12
	padding := 2
	sheet := image.NewRGBA(image.Rect(0, 0, cols*cellSize, rows*cellSize))

	for _, mItem := range ld.Manifest {
		tImg := ld.UniqueTiles[mItem.Hash]
		if tImg == nil {
			continue
		}
		gx := (mItem.Index % cols) * cellSize
		gy := (mItem.Index / cols) * cellSize

		// Tile central 8x8
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				sheet.Set(gx+padding+x, gy+padding+y, tImg.At(x, y))
			}
		}

		// Texture Gutter Clamping (márgenes anti-bleed)
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

	sf, err := os.Create(filepath.Join(ld.OutDir, "spritesheet.png"))
	if err == nil {
		png.Encode(sf, sheet)
		sf.Close()
	}

	var savedManifest interface{} = ld.Manifest
	if ld.Document != nil {
		document := *ld.Document
		document.Tiles = ld.Manifest
		savedManifest = document
		ld.Document = &document
	}
	saveJSONFile(filepath.Join(ld.OutDir, "manifest.json"), savedManifest)
	saveJSONFile(filepath.Join(ld.HdDir, "manifest.json"), savedManifest)

	saved := ld.NewTiles
	ld.NewTiles = 0
	return saved
}

func saveJSONFile(path string, data interface{}) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func (ld *LiveDumper) SaveScreenshot(img *ebiten.Image) string {
	ts := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("capture_%s.png", ts)
	path := filepath.Join(ld.ShotDir, filename)

	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}

	f, err := os.Create(path)
	if err == nil {
		png.Encode(f, rgba)
		f.Close()
		return filename
	}
	return ""
}
