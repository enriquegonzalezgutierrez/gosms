// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	"gosms/pkg/machine"
)

type levelTileCell struct {
	Tile          int    `json:"tile"`
	Palette       int    `json:"palette"`
	Hash          string `json:"hash"`
	ManifestIndex int    `json:"manifest_index"`
	Prefix        string `json:"prefix"`
	Row           int    `json:"row"`
	Column        int    `json:"column"`
	Archive       string `json:"archive"`
	Pack          string `json:"pack"`
}

// exportLevelTiles monta una hoja con los tiles reales de la VRAM estática del
// nivel. No usa original/tiles: esos índices son del manifest, no de la VRAM.
func exportLevelTiles(rom []byte, level int, outDir string) error {
	v, err := buildAlexKiddVRAM(rom, level)
	if err != nil {
		return err
	}
	scenes, _, err := alexKiddLayoutScenes(rom, filepath.Join(outDir, "scenes"))
	if err != nil {
		return err
	}
	table, err := alexKiddMetatileTable(rom, level)
	if err != nil {
		return err
	}

	type cellKey struct{ tile, palette int }
	used := make(map[cellKey]bool)
	for _, scene := range scenes {
		if scene.Level != level {
			continue
		}
		for _, row := range scene.MetatileMap {
			for _, metatile := range row {
				for _, entry := range alexKiddMetatileEntries(rom, table, metatile) {
					palette := 0
					if entry&0x800 != 0 {
						palette = 16
					}
					used[cellKey{tile: int(entry & 0x1ff), palette: palette}] = true
				}
			}
		}
	}
	keys := make([]cellKey, 0, len(used))
	for key := range used {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].tile != keys[j].tile {
			return keys[i].tile < keys[j].tile
		}
		return keys[i].palette < keys[j].palette
	})

	manifestPath := filepath.Join(outDir, "manifest.json")
	var document machine.TileManifestDocument
	if data, err := os.ReadFile(manifestPath); err == nil {
		_ = json.Unmarshal(data, &document)
	}
	byHash := make(map[string]machine.TileManifest, len(document.Tiles))
	for _, item := range document.Tiles {
		byHash[item.Hash] = item
	}

	const columns = 12
	const cell = 8
	const gutter = 2
	const scale = 8
	rows := (len(keys) + columns - 1) / columns
	step := cell + gutter
	sheet := image.NewRGBA(image.Rect(0, 0, columns*step*scale, rows*step*scale))
	for i := range sheet.Pix {
		sheet.Pix[i] = 0xff
		if i%4 == 3 {
			sheet.Pix[i] = 0xff
		}
	}
	// Fondo gris y líneas magenta en los márgenes.
	for y := 0; y < sheet.Bounds().Dy(); y++ {
		for x := 0; x < sheet.Bounds().Dx(); x++ {
			offset := y*sheet.Stride + x*4
			sheet.Pix[offset] = 40
			sheet.Pix[offset+1] = 40
			sheet.Pix[offset+2] = 45
		}
	}
	for c := 0; c <= columns; c++ {
		x := c * step * scale
		for y := 0; y < sheet.Bounds().Dy(); y++ {
			for t := 0; t < 2 && x+t < sheet.Bounds().Dx(); t++ {
				offset := y*sheet.Stride + (x+t)*4
				sheet.Pix[offset] = 255
				sheet.Pix[offset+1] = 0
				sheet.Pix[offset+2] = 255
			}
		}
	}
	for r := 0; r <= rows; r++ {
		y := r * step * scale
		for x := 0; x < sheet.Bounds().Dx(); x++ {
			for t := 0; t < 2 && y+t < sheet.Bounds().Dy(); t++ {
				offset := (y+t)*sheet.Stride + x*4
				sheet.Pix[offset] = 255
				sheet.Pix[offset+1] = 0
				sheet.Pix[offset+2] = 255
			}
		}
	}

	cells := make([]levelTileCell, 0, len(keys))
	for i, key := range keys {
		row, column := i/columns, i%columns
		tileData := v.tiles[key.tile*32 : key.tile*32+32]
		cram := v.cram[key.palette : key.palette+16]
		hash := machine.TileHash(tileData, cram, false)
		item, exists := byHash[hash]
		prefix := "bg"
		manifestIndex := -1
		if exists {
			manifestIndex = item.Index
			if item.IsSprite {
				prefix = "spr"
			}
		}
		palette := make([][3]byte, 16)
		for p := 0; p < 16; p++ {
			r, g, b := decodeStaticColor(cram[p])
			palette[p] = [3]byte{r, g, b}
		}
		// drawStaticTile usa stride de 256 (pantalla completa). Se dibuja en
		// un lienzo de 256x192 y se copia la esquina de 8x8.
		canvas := image.NewRGBA(image.Rect(0, 0, 256, 192))
		drawStaticTile(canvas.Pix, tileData, palette, 0, 0, false, false)
		tile := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			copy(tile.Pix[y*tile.Stride:y*tile.Stride+32], canvas.Pix[y*canvas.Stride:y*canvas.Stride+32])
		}
		// Escalar 8x con nearest y pegar en la celda.
		baseX := (column*step + gutter) * scale
		baseY := (row*step + gutter) * scale
		for y := 0; y < 8*scale; y++ {
			for x := 0; x < 8*scale; x++ {
				src := tile.Pix[(y/scale)*tile.Stride+(x/scale)*4:]
				dst := sheet.Pix[(baseY+y)*sheet.Stride+(baseX+x)*4:]
				copy(dst[:4], src[:4])
			}
		}
		cell := levelTileCell{
			Tile: key.tile, Palette: key.palette, Hash: hash,
			ManifestIndex: manifestIndex, Prefix: prefix,
			Row: row, Column: column,
		}
		if manifestIndex >= 0 {
			name := fmt.Sprintf("%s_%04d.png", prefix, manifestIndex)
			cell.Archive = filepath.ToSlash(filepath.Join("ai-remaster", "tiles", name))
			cell.Pack = filepath.ToSlash(filepath.Join("remaster_workspace", "tiles", name))
		}
		cells = append(cells, cell)
	}

	geminiDir := filepath.Join(filepath.Dir(outDir), "ai-remaster", "gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		return err
	}
	sheetPath := filepath.Join(geminiDir, fmt.Sprintf("level%02d_tiles.png", level))
	file, err := os.Create(sheetPath)
	if err != nil {
		return err
	}
	err = png.Encode(file, sheet)
	_ = file.Close()
	if err != nil {
		return err
	}
	documentPath := filepath.Join(geminiDir, fmt.Sprintf("level%02d_tiles.json", level))
	output, err := os.Create(documentPath)
	if err != nil {
		return err
	}
	defer output.Close()
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	meta := map[string]interface{}{
		"level": level, "scale": scale, "cell": cell, "gutter": gutter,
		"columns": columns, "rows": rows, "tiles": cells,
	}
	if err := encoder.Encode(meta); err != nil {
		return err
	}
	fmt.Printf("🧱 Hoja de tiles del nivel %d: %s (%d tiles, %dx%d)\n", level, sheetPath, len(cells), sheet.Bounds().Dx(), sheet.Bounds().Dy())
	return nil
}
