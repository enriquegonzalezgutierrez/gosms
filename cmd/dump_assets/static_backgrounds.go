// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"gosms/pkg/machine"
)

// alexKiddVRAM is the 16 KiB tile pattern memory reconstructed for one level.
// Metatile entries address these tiles by their 9-bit tile index.
type alexKiddVRAM struct {
	tiles []byte // 512 tiles * 32 bytes
	cram  []byte // 32 bytes: 16 background + 16 sprite colours
}

func (v *alexKiddVRAM) fill(addr int, value byte, count int) {
	base := addr & 0x3fff
	for i := 0; i < count && base+i < len(v.tiles); i++ {
		v.tiles[base+i] = value
	}
}

func (v *alexKiddVRAM) place(tiles [][32]byte, addr int) {
	base := (addr & 0x3fff) / 32
	for i, tile := range tiles {
		if base+i >= 512 {
			break
		}
		copy(v.tiles[(base+i)*32:], tile[:])
	}
}

func decodeStaticSet(rom []byte, id string) ([][32]byte, error) {
	for _, set := range alexKiddStaticTileSets {
		if set.id != id {
			continue
		}
		switch {
		case set.raw3BPP:
			return decodeRaw3BPP(rom, set.start, set.end)
		case set.raw4BPP:
			return decodeRaw4BPP(rom, set.start, set.end)
		default:
			return decodePhantasyStarTiles(rom, set.start, set.end)
		}
	}
	return nil, fmt.Errorf("tileset %s no encontrado", id)
}

func (v *alexKiddVRAM) load(rom []byte, id string, addr int) error {
	tiles, err := decodeStaticSet(rom, id)
	if err != nil {
		return err
	}
	v.place(tiles, addr)
	return nil
}

// buildAlexKiddVRAM reproduces the per-level tile loading performed by
// tilesetLoaders.asm: boxes and the coin bag are global, then the level main
// set and its additional sets are copied to their VRAM addresses.
func buildAlexKiddVRAM(rom []byte, level int) (*alexKiddVRAM, error) {
	if len(rom) != 128*1024 {
		return nil, fmt.Errorf("Alex Kidd estático requiere una ROM de 128 KiB")
	}
	v := &alexKiddVRAM{tiles: make([]byte, 512*32)}
	if err := v.load(rom, "box_icons", 0x4020); err != nil {
		return nil, err
	}
	if err := v.load(rom, "shared_coins_cloud", 0x44a0); err != nil {
		return nil, err
	}
	pointer := int(rom[0x1112+(level-1)*2]) | int(rom[0x1113+(level-1)*2])<<8
	offset := 0x14000 + pointer
	if offset < 0 || offset+32 > len(rom) {
		return nil, fmt.Errorf("paleta del nivel %d fuera de la ROM", level)
	}
	v.cram = append([]byte(nil), rom[offset:offset+32]...)

	switch level {
	case 1:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_1_5_9", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_4_raw", 0x4e60); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_1", 0x4f00); err != nil {
			return nil, err
		}
	case 2, 13:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_2_6_10_12_13_15", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_2", 0x4ec0); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_3", 0x47a0); err != nil {
			return nil, err
		}
	case 3:
		v.fill(0x46a0, 0, 0x200)
		if err := v.load(rom, "main_3_17", 0x48a0); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_5", 0x4d00); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_4_raw", 0x4e60); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_1", 0x4f00); err != nil {
			return nil, err
		}
	case 4:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_4", 0x4720); err != nil {
			return nil, err
		}
	case 5:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_1_5_9", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_5", 0x4d00); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_3", 0x47a0); err != nil {
			return nil, err
		}
	case 6, 12:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_2_6_10_12_13_15", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "main_6_12", 0x4920); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_2", 0x4ec0); err != nil {
			return nil, err
		}
	case 7:
		v.fill(0x46a0, 0x0a, 0x80)
		if err := v.load(rom, "main_7", 0x4720); err != nil {
			return nil, err
		}
	case 8:
		if err := v.load(rom, "additional_2", 0x4ec0); err != nil {
			return nil, err
		}
		if err := v.load(rom, "main_8", 0x46a0); err != nil {
			return nil, err
		}
	case 9:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_1_5_9", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_5", 0x4d00); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_1", 0x4f00); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_3", 0x47a0); err != nil {
			return nil, err
		}
	case 10, 15:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_2_6_10_12_13_15", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_2", 0x4ec0); err != nil {
			return nil, err
		}
	case 11:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_11_14", 0x4720); err != nil {
			return nil, err
		}
	case 14:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_11_14", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "main_14", 0x4da0); err != nil {
			return nil, err
		}
	case 16:
		v.fill(0x46a0, 0, 0x80)
		if err := v.load(rom, "main_16", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_1", 0x4f00); err != nil {
			return nil, err
		}
	case 17:
		v.fill(0x46a0, 0, 0x200)
		if err := v.load(rom, "main_3_17", 0x48a0); err != nil {
			return nil, err
		}
		if err := v.load(rom, "main_17", 0x4aa0); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_17", 0x4720); err != nil {
			return nil, err
		}
		if err := v.load(rom, "additional_1", 0x4f00); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("nivel Alex Kidd inválido %d", level)
	}
	return v, nil
}

func alexKiddMetatileTable(rom []byte, level int) (int, error) {
	descriptor := int(rom[0x66d6+(level-1)*2]) | int(rom[0x66d7+(level-1)*2])<<8
	if descriptor < 0 || descriptor+12 > len(rom) {
		return 0, fmt.Errorf("descriptor del nivel %d fuera de la ROM", level)
	}
	pointer := int(rom[descriptor+10]) | int(rom[descriptor+11])<<8
	offset := 0x14000 + pointer - 0x8000
	if offset < 0x14000 || offset >= 0x14400 {
		return 0, fmt.Errorf("tabla de metatiles del nivel %d inválida (%#x)", level, offset)
	}
	return offset, nil
}

func alexKiddMetatileEntries(rom []byte, table, index int) [4]uint16 {
	var entries [4]uint16
	pointer := int(rom[table+index*2]) | int(rom[table+index*2+1])<<8
	offset := 0x14000 + pointer - 0x8000
	if offset < 0x14450 || offset+8 > 0x14d35 {
		return entries
	}
	for i := range entries {
		entries[i] = uint16(rom[offset+i*2]) | uint16(rom[offset+i*2+1])<<8
	}
	return entries
}

func decodeStaticColor(c byte) (byte, byte, byte) {
	return (c & 3) * 85, ((c >> 2) & 3) * 85, ((c >> 4) & 3) * 85
}

// renderAlexKiddScreen renders one 16x12 metatile screen into a 256x192 image
// and returns the per-cell signature used by the runtime object matcher.
func renderAlexKiddScreen(rom []byte, v *alexKiddVRAM, scene machine.TileSceneManifest) (*image.RGBA, []machine.TileObjectPattern, error) {
	table, err := alexKiddMetatileTable(rom, scene.Level)
	if err != nil {
		return nil, nil, err
	}
	if len(scene.MetatileMap) != 12 {
		return nil, nil, fmt.Errorf("pantalla del nivel %d sin 12 filas de metatiles", scene.Level)
	}
	img := image.NewRGBA(image.Rect(0, 0, 256, 192))
	pixels := img.Pix
	palette := make([][3]byte, 32)
	for i := 0; i < 32; i++ {
		r, g, b := decodeStaticColor(v.cram[i])
		palette[i] = [3]byte{r, g, b}
	}
	var cells [24][32]uint16
	for my := 0; my < 12; my++ {
		row := scene.MetatileMap[my]
		if len(row) != 16 {
			return nil, nil, fmt.Errorf("pantalla del nivel %d sin 16 columnas de metatiles", scene.Level)
		}
		for mx := 0; mx < 16; mx++ {
			entries := alexKiddMetatileEntries(rom, table, row[mx])
			// The four entries are stored as [top-left, bottom-left,
			// top-right, bottom-right].
			cells[my*2][mx*2] = entries[0]
			cells[my*2+1][mx*2] = entries[1]
			cells[my*2][mx*2+1] = entries[2]
			cells[my*2+1][mx*2+1] = entries[3]
			for i, entry := range entries {
				tile := int(entry & 0x1ff)
				hflip := entry&0x200 != 0
				vflip := entry&0x400 != 0
				palOffset := 0
				if entry&0x800 != 0 {
					palOffset = 16
				}
				px := mx*16 + (8 * (i / 2))
				py := my*16 + (8 * (i % 2))
				drawStaticTile(pixels, v.tiles[tile*32:tile*32+32], palette[palOffset:palOffset+16], px, py, hflip, vflip)
			}
		}
	}
	signature := make([]machine.TileObjectPattern, 0, 24*32)
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			entry := cells[y][x]
			tile := int(entry & 0x1ff)
			palOffset := 0
			if entry&0x800 != 0 {
				palOffset = 16
			}
			hash := machine.CellHash(v.tiles[tile*32:tile*32+32], v.cram[palOffset:palOffset+16], entry&0x200 != 0, entry&0x400 != 0)
			signature = append(signature, machine.TileObjectPattern{DX: x, DY: y, Hash: hash})
		}
	}
	return img, signature, nil
}

func drawStaticTile(dst []byte, tile []byte, palette [][3]byte, px, py int, hflip, vflip bool) {
	for y := 0; y < 8; y++ {
		b0, b1, b2, b3 := tile[y*4], tile[y*4+1], tile[y*4+2], tile[y*4+3]
		for x := 0; x < 8; x++ {
			bit := uint(7 - x)
			color := int(((b0 >> bit) & 1) | (((b1 >> bit) & 1) << 1) | (((b2 >> bit) & 1) << 2) | (((b3 >> bit) & 1) << 3))
			sx := x
			if hflip {
				sx = 7 - x
			}
			sy := y
			if vflip {
				sy = 7 - y
			}
			offset := ((py+sy)*256 + px + sx) * 4
			if offset < 0 || offset+3 >= len(dst) {
				continue
			}
			c := palette[color&0x0f]
			dst[offset] = c[0]
			dst[offset+1] = c[1]
			dst[offset+2] = c[2]
			dst[offset+3] = 255
		}
	}
}

// alexKiddBackgroundObjects renders every static 256x192 screen and returns
// complete background objects. The anchor is the least frequent cell in the
// whole catalogue, which keeps the runtime index small.
func alexKiddBackgroundObjects(rom []byte, scenes []machine.TileSceneManifest, objectDir string) ([]*objectAccumulator, error) {
	type rendered struct {
		scene     machine.TileSceneManifest
		img       *image.RGBA
		signature []machine.TileObjectPattern
		id        string
	}
	renderedScenes := make([]rendered, 0, len(scenes))
	frequencies := make(map[string]int)
	for _, scene := range scenes {
		v, err := buildAlexKiddVRAM(rom, scene.Level)
		if err != nil {
			return nil, err
		}
		img, signature, err := renderAlexKiddScreen(rom, v, scene)
		if err != nil {
			return nil, err
		}
		h := fnv.New64a()
		_, _ = fmt.Fprintf(h, "bg_screen:%d:%d;", scene.Level, scene.ROMOffset)
		id := fmt.Sprintf("bg_screen_%016x", h.Sum64())
		for _, pattern := range signature {
			frequencies[pattern.Hash]++
		}
		renderedScenes = append(renderedScenes, rendered{scene: scene, img: img, signature: signature, id: id})
	}

	objects := make([]*objectAccumulator, 0, len(renderedScenes))
	for _, item := range renderedScenes {
		anchorIndex := 0
		best := frequencies[item.signature[0].Hash]
		for i, pattern := range item.signature {
			if frequencies[pattern.Hash] < best {
				best = frequencies[pattern.Hash]
				anchorIndex = i
			}
		}
		anchor := item.signature[anchorIndex]
		label := fmt.Sprintf("Fondo %s · pantalla %d", item.scene.LevelName, item.scene.Screen)
		manifest := machine.TileObjectManifest{
			ID:         item.id,
			Kind:       "background_screen",
			Label:      label,
			Preview:    filepath.ToSlash(filepath.Join("objects", item.id+".png")),
			FramesSeen: 1,
			FirstFrame: 0,
			Width:      256,
			Height:     192,
			Anchor:     &anchor,
			Signature:  item.signature,
		}
		if err := writeObjectPNG(objectDir, item.id, item.img); err != nil {
			return nil, err
		}
		objects = append(objects, &objectAccumulator{manifest: manifest, preview: item.img})
	}
	return objects, nil
}

func writeObjectPNG(dir, id string, img image.Image) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(dir, id+".png"))
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}
