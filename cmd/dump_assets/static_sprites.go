// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"hash/fnv"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gosms/pkg/machine"
)

// --- Poses estáticas de Alex ---------------------------------------------

type spritePosePart struct {
	X, Y int
	Tile int
	Data [32]byte
}

type spritePoseDef struct {
	Label  string
	Offset int
	Parts  []spritePosePart
	Width  int
	Height int
}

// alexKiddPoseDescriptors son los descriptores de sprite de Alex localizados
// en spriteDescriptors.asm. Las animaciones periódicas repiten algunos frames
// (andar 1/3, por ejemplo); la deduplicación por firma los une.
var alexKiddPoseDescriptors = []struct {
	label  string
	offset int
}{
	{"Alex quieto izquierda", 0x90A7},
	{"Alex quieto derecha", 0x90BC},
	{"Alex andando izquierda 1", 0x8D2F},
	{"Alex andando izquierda 2", 0x8D41},
	{"Alex andando izquierda 3", 0x8D2F},
	{"Alex andando izquierda 4", 0x8D56},
	{"Alex andando derecha 1", 0x8D6B},
	{"Alex andando derecha 2", 0x8D7D},
	{"Alex andando derecha 3", 0x8D6B},
	{"Alex andando derecha 4", 0x8D92},
	{"Alex nadando izquierda 1", 0x8E01},
	{"Alex nadando izquierda 2", 0x8E13},
	{"Alex nadando derecha 1", 0x8E25},
	{"Alex nadando derecha 2", 0x8E37},
	{"Alex peticóptero izquierda 1", 0x8F7B},
	{"Alex peticóptero izquierda 2", 0x8F9F},
	{"Alex peticóptero izquierda 3", 0x8FC6},
	{"Alex peticóptero izquierda 4", 0x8FEA},
	{"Alex peticóptero derecha 1", 0x9011},
	{"Alex peticóptero derecha 2", 0x9035},
	{"Alex peticóptero derecha 3", 0x905C},
	{"Alex peticóptero derecha 4", 0x9080},
	{"Alex moto 1", 0x8F2A},
	{"Alex moto 2", 0x8F45},
	{"Alex barca 1", 0x9152},
	{"Alex barca 2", 0x916D},
	{"Alex muerto 1", 0x90E3},
	{"Alex muerto 2", 0x90F8},
	{"Alex muerto 3", 0x910D},
	{"Alex agachado izquierda", 0x8DA7},
	{"Alex agachado derecha", 0x8DBC},
	{"Alex puñetazo izquierda", 0x8DD1},
	{"Alex puñetazo derecha", 0x8DE9},
	{"Alex aire izquierda", 0x8F00},
	{"Alex aire derecha", 0x8F15},
}

func readLE16(data []byte, offset int) int {
	if offset < 0 || offset+1 >= len(data) {
		return -1
	}
	return int(data[offset]) | int(data[offset+1])<<8
}

// alexKiddStaticTileSet decodifica los tiles 3bpp de una entrada de la tabla
// de alexFrameDescriptors. Cada tile se expande a 4 planos para poder usar el
// mismo hash y el mismo renderizador que el resto del emulador.
func alexKiddStaticTileSet(rom []byte, index int) ([][32]byte, error) {
	if index < 0 || index >= 48 {
		return nil, fmt.Errorf("tile set de Alex fuera de rango: %d", index)
	}
	table := 0x10000 + index*2
	pointer := readLE16(rom, table)
	if pointer < 0 || pointer >= len(rom) {
		return nil, fmt.Errorf("puntero de tile set de Alex inválido: %#x", pointer)
	}
	// La tabla vive en el banco 4 ($10000) y contiene direcciones de CPU del
	// slot 2 ($8000-$BFFF); el offset plano es banco + (dirección - $8000).
	descriptor := 0x10000 + (pointer & 0x3FFF)
	if descriptor < 0 || descriptor >= len(rom) {
		return nil, fmt.Errorf("descriptor de tiles de Alex fuera de ROM: %#x", descriptor)
	}
	count := int(rom[descriptor])
	if count <= 0 || count > 32 {
		return nil, fmt.Errorf("tile set de Alex con %d tiles", count)
	}
	tiles := make([][32]byte, 0, count)
	for i := 0; i < count; i++ {
		tilePointer := readLE16(rom, descriptor+1+i*2)
		if tilePointer < 0 {
			return nil, fmt.Errorf("tile de Alex fuera de ROM: %#x", tilePointer)
		}
		offset := tilePointer
		if tilePointer >= 0x8000 && tilePointer < 0xC000 {
			offset = 0x10000 + (tilePointer & 0x3FFF)
		}
		if offset < 0 || offset+24 > len(rom) {
			return nil, fmt.Errorf("tile de Alex fuera de ROM: %#x", offset)
		}
		decoded, err := decodeRaw3BPP(rom, offset, offset+24)
		if err != nil {
			return nil, err
		}
		tiles = append(tiles, decoded[0])
	}
	return tiles, nil
}

func alexKiddSpritePose(rom []byte, label string, offset int) (spritePoseDef, error) {
	def := spritePoseDef{Label: label, Offset: offset}
	if offset < 0 || offset+3 > len(rom) {
		return def, fmt.Errorf("descriptor de pose fuera de ROM: %#x", offset)
	}
	tileSet := int(rom[offset] & 0x3F)
	count := int(rom[offset+1])
	if count <= 0 || count > 32 {
		return def, fmt.Errorf("pose %q con %d sprites", label, count)
	}
	tiles, err := alexKiddStaticTileSet(rom, tileSet)
	if err != nil {
		return def, fmt.Errorf("pose %q: %w", label, err)
	}
	cursor := offset + 3 // byte 2 son flags/atributos del descriptor
	if cursor+count > len(rom) {
		return def, fmt.Errorf("pose %q sin offsets Y", label)
	}
	ys := rom[cursor : cursor+count]
	cursor += count
	if cursor+count*2 > len(rom) {
		return def, fmt.Errorf("pose %q sin pares X/tile", label)
	}
	maxX, maxY := 0, 0
	minX, minY := 0, 0
	for i := 0; i < count; i++ {
		// Los offsets son bytes con signo: el puñetazo izquierdo extiende el
		// puño a X negativa.
		x := int(int8(rom[cursor+i*2]))
		tile := int(rom[cursor+i*2+1])
		if tile >= len(tiles) {
			return def, fmt.Errorf("pose %q referencia el tile %d de %d", label, tile, len(tiles))
		}
		y := int(int8(ys[i]))
		def.Parts = append(def.Parts, spritePosePart{X: x, Y: y, Tile: tile, Data: tiles[tile]})
		if x+8 > maxX {
			maxX = x + 8
		}
		if y+8 > maxY {
			maxY = y + 8
		}
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
	}
	for i := range def.Parts {
		def.Parts[i].X -= minX
		def.Parts[i].Y -= minY
	}
	def.Width, def.Height = maxX-minX, maxY-minY
	return def, nil
}

func drawSpritePoseTile(dst []byte, width int, tile [32]byte, palette [][3]byte, px, py int) {
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			bit := uint(7 - x)
			color := int(((tile[y*4] >> bit) & 1) | (((tile[y*4+1] >> bit) & 1) << 1) | (((tile[y*4+2] >> bit) & 1) << 2) | (((tile[y*4+3] >> bit) & 1) << 3))
			if color&0x0F == 0 {
				continue
			}
			offset := ((py+y)*width + px + x) * 4
			if offset < 0 || offset+3 >= len(dst) {
				continue
			}
			c := palette[color&0x0F]
			dst[offset] = c[0]
			dst[offset+1] = c[1]
			dst[offset+2] = c[2]
			dst[offset+3] = 255
		}
	}
}

func spritePoseSignature(parts []spritePosePart, cram []byte) []machine.TileObjectPattern {
	signature := make([]machine.TileObjectPattern, 0, len(parts))
	for _, part := range parts {
		if isBlankTile(part.Data[:]) {
			continue
		}
		signature = append(signature, machine.TileObjectPattern{
			DX: part.X, DY: part.Y, Hash: machine.TileHash(part.Data[:], cram, true),
		})
	}
	return signature
}

func spritePoseID(signature []machine.TileObjectPattern) string {
	parts := make([]string, 0, len(signature))
	for _, p := range signature {
		parts = append(parts, fmt.Sprintf("%s@%d,%d", p.Hash, p.DX, p.DY))
	}
	sort.Strings(parts)
	h := fnv.New64a()
	for _, part := range parts {
		_, _ = h.Write([]byte(part + ";"))
	}
	return fmt.Sprintf("sprite_pose_%016x", h.Sum64())
}

// alexKiddSpritePoseObjects renderiza todas las poses estáticas de Alex para
// cada paleta de sprites distinta y las deja como objetos completos. La imagen
// es la pose entera; la firma permite reconocerla en runtime sin depender del
// orden de la SAT.
func alexKiddSpritePoseObjects(rom []byte, objectDir string) ([]*objectAccumulator, error) {
	palettes := make(map[string][]byte)
	for level := 1; level <= 17; level++ {
		cram, err := levelCRAM(rom, level, true)
		if err != nil {
			return nil, err
		}
		palettes[string(cram)] = cram
	}

	type variant struct {
		label     string
		image     *image.RGBA
		signature []machine.TileObjectPattern
		width     int
		height    int
	}
	bySignature := make(map[string]*variant)
	for _, descriptor := range alexKiddPoseDescriptors {
		pose, err := alexKiddSpritePose(rom, descriptor.label, descriptor.offset)
		if err != nil {
			return nil, err
		}
		for _, cram := range palettes {
			signature := spritePoseSignature(pose.Parts, cram)
			if len(signature) == 0 {
				continue
			}
			id := spritePoseID(signature)
			if _, exists := bySignature[id]; exists {
				continue
			}
			img := image.NewRGBA(image.Rect(0, 0, pose.Width, pose.Height))
			palette := make([][3]byte, 16)
			for i := 0; i < 16; i++ {
				r, g, b := decodeStaticColor(cram[i])
				palette[i] = [3]byte{r, g, b}
			}
			for _, part := range pose.Parts {
				drawSpritePoseTile(img.Pix, pose.Width, part.Data, palette, part.X, part.Y)
			}
			bySignature[id] = &variant{
				label: pose.Label, image: img, signature: signature,
				width: pose.Width, height: pose.Height,
			}
		}
	}

	frequencies := make(map[string]int)
	for _, item := range bySignature {
		for _, pattern := range item.signature {
			frequencies[pattern.Hash]++
		}
	}

	ids := make([]string, 0, len(bySignature))
	for id := range bySignature {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	objects := make([]*objectAccumulator, 0, len(ids))
	for _, id := range ids {
		item := bySignature[id]
		anchorIndex := 0
		best := frequencies[item.signature[0].Hash]
		for i, pattern := range item.signature {
			if frequencies[pattern.Hash] < best {
				best = frequencies[pattern.Hash]
				anchorIndex = i
			}
		}
		anchor := item.signature[anchorIndex]
		if err := writeObjectPNG(objectDir, id, item.image); err != nil {
			return nil, err
		}
		objects = append(objects, &objectAccumulator{
			manifest: machine.TileObjectManifest{
				ID: id, Kind: "sprite_pose", Label: item.label,
				Preview:    filepath.ToSlash(filepath.Join("objects", id+".png")),
				FramesSeen: 1, FirstFrame: 0,
				Width: item.width, Height: item.height,
				Anchor: &anchor, Signature: item.signature,
			},
			preview: item.image,
		})
	}
	return objects, nil
}

// pruneObjectPNGs borra los previews que ya no referencia el manifest para
// que original/objects solo contenga objetos completos vivos.
func pruneObjectPNGs(objectDir string, objects []machine.TileObjectManifest) {
	keep := make(map[string]bool, len(objects))
	for _, obj := range objects {
		keep[obj.ID+".png"] = true
	}
	entries, err := os.ReadDir(objectDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".png") {
			continue
		}
		if !keep[entry.Name()] {
			_ = os.Remove(filepath.Join(objectDir, entry.Name()))
		}
	}
}
