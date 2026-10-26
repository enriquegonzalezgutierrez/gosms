package main

import (
	"fmt"
	"image"
	"image/draw"

	"gosms/pkg/machine"
)

type staticTileSet struct {
	id      string
	start   int
	end     int
	levels  []int
	sprite  bool
	raw3BPP bool
	raw4BPP bool
	label   string
}

var alexKiddStaticTileSets = []staticTileSet{
	{id: "box_icons", start: 0x0c000, end: 0x0c480, levels: rangeLevels(1, 17), raw4BPP: true, label: "cajas y objetos"},
	{id: "shared_coins_cloud", start: 0x0c4a2, end: 0x0c583, levels: rangeLevels(1, 17), label: "monedas y nube"},
	{id: "additional_1", start: 0x0c583, end: 0x0c9e1, levels: []int{1, 3, 5, 9, 16, 17}, label: "tiles adicionales compartidos"},
	{id: "additional_2", start: 0x0c9e1, end: 0x0ce65, levels: []int{2, 6, 8, 10, 12, 13, 15}, label: "tiles adicionales compartidos"},
	{id: "additional_3", start: 0x0ce65, end: 0x0cece, levels: []int{2, 5, 9, 13}, label: "tiles adicionales compartidos"},
	{id: "main_1_5_9", start: 0x0cece, end: 0x0d158, levels: []int{1, 5, 9}, label: "tiles del nivel"},
	{id: "main_2_6_10_12_13_15", start: 0x0d158, end: 0x0d317, levels: []int{2, 6, 10, 12, 13, 15}, label: "tiles del nivel"},
	{id: "main_3_17", start: 0x0d317, end: 0x0d56a, levels: []int{3, 17}, label: "tiles del nivel"},
	{id: "main_4", start: 0x0d56a, end: 0x0db82, levels: []int{4}, label: "tiles del nivel"},
	{id: "main_6_12", start: 0x0db82, end: 0x0df1d, levels: []int{6, 12}, label: "tiles del nivel"},
	{id: "main_7", start: 0x0df1d, end: 0x0e24c, levels: []int{7}, label: "tiles del nivel"},
	{id: "main_8", start: 0x0e24c, end: 0x0ea05, levels: []int{8}, label: "tiles del nivel"},
	{id: "main_11_14", start: 0x0ea05, end: 0x0ee33, levels: []int{11, 14}, label: "tiles del nivel"},
	{id: "main_16", start: 0x0ee33, end: 0x0f117, levels: []int{16}, label: "tiles del nivel"},
	{id: "main_17", start: 0x0f117, end: 0x0f49b, levels: []int{17}, label: "tiles del nivel"},
	{id: "main_14", start: 0x0f49b, end: 0x0f75c, levels: []int{14}, label: "tiles del nivel"},
	{id: "additional_17", start: 0x0f75c, end: 0x0f7f6, levels: []int{17}, label: "tiles adicionales"},
	{id: "additional_4_raw", start: 0x0f7f6, end: 0x0f896, levels: []int{1, 3, 5}, raw4BPP: true, label: "tiles adicionales"},
	{id: "additional_5", start: 0x0f896, end: 0x0f96a, levels: []int{3, 5, 9}, label: "tiles adicionales compartidos"},
	{id: "alex_kidd_3bpp", start: 0x10341, end: 0x118e9, levels: rangeLevels(1, 17), sprite: true, raw3BPP: true, label: "Alex Kidd"},
	{id: "janken_match", start: 0x118e9, end: 0x11bb5, levels: rangeLevels(1, 17), sprite: true, label: "combate de janken"},
	{id: "stone_head", start: 0x11bb5, end: 0x11e67, levels: rangeLevels(1, 17), sprite: true, label: "cabeza de piedra"},
	{id: "scissors_head", start: 0x11e67, end: 0x120a8, levels: rangeLevels(1, 17), sprite: true, label: "cabeza de tijeras"},
	{id: "paper_head", start: 0x120a8, end: 0x12357, levels: rangeLevels(1, 17), sprite: true, label: "cabeza de papel"},
	{id: "janken_great", start: 0x12357, end: 0x12ac2, levels: rangeLevels(1, 17), sprite: true, label: "Janken the Great"},
}

func rangeLevels(first, last int) []int {
	levels := make([]int, last-first+1)
	for i := range levels {
		levels[i] = first + i
	}
	return levels
}

func decodePhantasyStarTiles(rom []byte, start, end int) ([][32]byte, error) {
	if start < 0 || end > len(rom) || start >= end {
		return nil, fmt.Errorf("rango de tiles inválido %#x-%#x", start, end)
	}
	cursor := start
	planes := make([][]byte, 4)
	for plane := range planes {
		for {
			if cursor >= end {
				return nil, fmt.Errorf("stream %#x truncado en el plano %d", start, plane)
			}
			control := rom[cursor]
			cursor++
			if control == 0 {
				break
			}
			count := int(control & 0x7f)
			if count == 0 {
				return nil, fmt.Errorf("control RLE inválido en %#x", cursor-1)
			}
			if control&0x80 != 0 {
				if cursor+count > end {
					return nil, fmt.Errorf("literal RLE truncado en %#x", cursor)
				}
				planes[plane] = append(planes[plane], rom[cursor:cursor+count]...)
				cursor += count
			} else {
				if cursor >= end {
					return nil, fmt.Errorf("run RLE truncado en %#x", cursor)
				}
				value := rom[cursor]
				cursor++
				for i := 0; i < count; i++ {
					planes[plane] = append(planes[plane], value)
				}
			}
		}
	}
	length := len(planes[0])
	for plane := 1; plane < 4; plane++ {
		if len(planes[plane]) < length {
			length = len(planes[plane])
		}
	}
	length -= length % 8
	tiles := make([][32]byte, length/8)
	for tileIndex := range tiles {
		for row := 0; row < 8; row++ {
			for plane := 0; plane < 4; plane++ {
				tiles[tileIndex][row*4+plane] = planes[plane][tileIndex*8+row]
			}
		}
	}
	return tiles, nil
}

func decodeRaw3BPP(rom []byte, start, end int) ([][32]byte, error) {
	if start < 0 || end > len(rom) || start >= end || (end-start)%24 != 0 {
		return nil, fmt.Errorf("rango 3bpp inválido %#x-%#x", start, end)
	}
	tiles := make([][32]byte, (end-start)/24)
	for tileIndex := range tiles {
		for row := 0; row < 8; row++ {
			for plane := 0; plane < 3; plane++ {
				tiles[tileIndex][row*4+plane] = rom[start+tileIndex*24+row*3+plane]
			}
		}
	}
	return tiles, nil
}

func decodeRaw4BPP(rom []byte, start, end int) ([][32]byte, error) {
	if start < 0 || end > len(rom) || start >= end || (end-start)%32 != 0 {
		return nil, fmt.Errorf("rango 4bpp inválido %#x-%#x", start, end)
	}
	tiles := make([][32]byte, (end-start)/32)
	for i := range tiles {
		copy(tiles[i][:], rom[start+i*32:start+(i+1)*32])
	}
	return tiles, nil
}

func levelCRAM(rom []byte, level int, sprite bool) ([]byte, error) {
	if level < 1 || level > 17 {
		return nil, fmt.Errorf("nivel de paleta inválido %d", level)
	}
	pointer := int(rom[0x1112+(level-1)*2]) | int(rom[0x1113+(level-1)*2])<<8
	offset := 0x14000 + pointer - 0x8000
	if offset < 0 || offset+32 > len(rom) {
		return nil, fmt.Errorf("paleta del nivel %d fuera de la ROM", level)
	}
	cram := append([]byte(nil), rom[offset:offset+32]...)
	if sprite {
		return cram[16:], nil
	}
	return cram[:16], nil
}

func addStaticTileSets(rom []byte, level int, addTile func([]byte, []byte, bool) (int, string), tileImages map[string]*image.RGBA, objects map[string]*objectAccumulator) error {
	for _, set := range alexKiddStaticTileSets {
		used := false
		for _, setLevel := range set.levels {
			if setLevel == level {
				used = true
				break
			}
		}
		if !used {
			continue
		}
		var decoded [][32]byte
		var err error
		if set.raw3BPP {
			decoded, err = decodeRaw3BPP(rom, set.start, set.end)
		} else if set.raw4BPP {
			decoded, err = decodeRaw4BPP(rom, set.start, set.end)
		} else {
			decoded, err = decodePhantasyStarTiles(rom, set.start, set.end)
		}
		if err != nil {
			return fmt.Errorf("tileset %s: %w", set.id, err)
		}
		cram, err := levelCRAM(rom, level, set.sprite)
		if err != nil {
			return err
		}
		cols := 16
		preview := image.NewRGBA(image.Rect(0, 0, cols*8, ((len(decoded)+cols-1)/cols)*8))
		placements := make([]machine.TileObjectPlacement, 0, len(decoded))
		for tileIndex, tileData := range decoded {
			index, hash := addTile(tileData[:], cram, set.sprite)
			if index < 0 {
				continue
			}
			x, y := (tileIndex%cols)*8, (tileIndex/cols)*8
			placements = append(placements, machine.TileObjectPlacement{Index: index, X: x, Y: y})
			draw.Draw(preview, image.Rect(x, y, x+8, y+8), tileImages[hash], image.Point{}, draw.Src)
		}
		if len(placements) == 0 {
			continue
		}
		id := fmt.Sprintf("rom_%s_level_%02d", set.id, level)
		objects[id] = &objectAccumulator{manifest: machine.TileObjectManifest{
			ID: id, Kind: map[bool]string{true: "sprite_tileset", false: "background_tileset"}[set.sprite],
			Label: set.label, Preview: fmt.Sprintf("objects/%s.png", id), FramesSeen: 1,
			FirstFrame: 0, Tiles: placements,
		}, preview: preview}
	}
	return nil
}
