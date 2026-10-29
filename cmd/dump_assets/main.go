// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gosms/pkg/machine"
	"gosms/pkg/vdp"
)

const extractionFrames = 6500
const alexKiddV1CRC32 = 0xaed9aac4

// Sonic 1 (Master System) stores the level to load at $D23E. Writing it while
// the title screen is active is the same trick the Action Replay level-select
// code uses, so each zone/act can be captured without playing the whole game.
const sonicLevelRAM = 0xD23E
const sonicLivesRAM = 0xD246

var sonicLevelNames = []string{
	"Green Hill 1", "Green Hill 2", "Green Hill 3",
	"Bridge 1", "Bridge 2", "Bridge 3",
	"Jungle 1", "Jungle 2", "Jungle 3",
	"Labyrinth 1", "Labyrinth 2", "Labyrinth 3",
	"Scrap Brain 1", "Scrap Brain 2", "Scrap Brain 3",
	"Sky Base 1", "Sky Base 2", "Sky Base 3",
}

// Castle of Illusion keeps the three hall doors as 6-byte entities starting
// at $C260. The first byte is the door index with bit 7 set (the pending-door
// flag). Replacing it lets us enter any of the six worlds without clearing the
// previous ones, which is what the normal game requires.
const castleDoorEntityRAM = 0xC260
const castleBootFrames = 4000

var castleLevelNames = []string{
	"Enchanted Forest", "Toy Land", "Dessert Factory",
	"Library", "Clock Tower", "The Castle",
}

// castlePad reproduces the deterministic intro used to reach the castle hall:
// skip the title, choose Normal, skip the story pages and walk to the first
// door. From castleBootFrames on, the regular extraction bot takes over.
func castlePad(frame int, bot string, bootFrames int) byte {
	if frame >= bootFrames {
		return inputForFrame(750+frame-bootFrames, bot)
	}
	pad := byte(0xFF)
	for _, start := range []int{700, 1000, 1200, 1500, 1700, 1900, 2100, 2300, 2500, 2700, 2900, 3100, 3300} {
		if frame >= start && frame < start+8 {
			pad &= 0xEF // Botón 1 / START
		}
	}
	if frame >= 1100 && frame < 1108 {
		pad &= 0xFD // Abajo: selecciona Normal
	}
	if frame >= 3400 && frame < 3445 {
		pad &= 0xF7 // Derecha hasta la primera puerta
	}
	if frame >= 3550 && frame < 3580 {
		pad &= 0xFE // Arriba con Mickey ya detenido
	}
	return pad
}

func castleSessions(frames int) []extractionSession {
	sessions := make([]extractionSession, len(castleLevelNames))
	for i, name := range castleLevelNames {
		sessions[i] = extractionSession{level: i, name: name, bootFrames: castleBootFrames, frames: castleBootFrames + frames}
	}
	return sessions
}

type extractionSession struct {
	level      int
	name       string
	bootFrames int
	frames     int
}

func sonicSessions(frames int) []extractionSession {
	sessions := make([]extractionSession, len(sonicLevelNames))
	for i, name := range sonicLevelNames {
		sessions[i] = extractionSession{level: i, name: name, frames: frames}
	}
	return sessions
}

func inputForFrame(frame int, bot string) byte {
	pad := byte(0xFF)
	if frame >= 300 && frame < 550 && (frame/12)%2 == 0 {
		pad &^= 0x10 // Start
	}
	if frame < 750 {
		return pad
	}
	if bot == "alex-kidd" {
		relative := frame - 750
		direction := (relative / 240) % 6
		switch direction {
		case 0, 1, 5:
			pad &^= 0x08 // Right
		case 2:
			pad &^= 0x09 // Up + right
		case 3:
			pad &^= 0x0A // Down + right
		case 4:
			pad &^= 0x04 // Left
		}
		buttonPhase := relative % 36
		if buttonPhase < 20 {
			pad &^= 0x30 // Jump + punch
		}
		return pad
	}
	pad &^= 0x08 // Right
	if (frame-750)%120 < 20 {
		pad &^= 0x10 // Jump
	}
	return pad
}

type visibleSprite struct {
	x, y, width, height int
	parts               []machine.TileObjectPlacement
}

type objectAccumulator struct {
	manifest machine.TileObjectManifest
	preview  *image.RGBA
}

type rawTile [32]byte

func indexROMTiles(rom []byte) map[rawTile][]int {
	index := make(map[rawTile][]int)
	for offset := 0; offset+32 <= len(rom); offset++ {
		var tile rawTile
		copy(tile[:], rom[offset:offset+32])
		blank := true
		for _, b := range tile {
			if b != 0 {
				blank = false
				break
			}
		}
		if !blank {
			if len(index[tile]) < 8 {
				index[tile] = append(index[tile], offset)
			}
		}
	}
	return index
}

func tileKey(data []byte) rawTile {
	var key rawTile
	copy(key[:], data)
	return key
}

func spritesTouch(a, b visibleSprite) bool {
	gap := func(a0, a1, b0, b1 int) int {
		if a1 < b0 {
			return b0 - a1
		}
		if b1 < a0 {
			return a0 - b1
		}
		return 0
	}
	return gap(a.x, a.x+a.width, b.x, b.x+b.width) <= 1 &&
		gap(a.y, a.y+a.height, b.y, b.y+b.height) <= 1
}

// groupSprites decides which visible sprites become a single remaster object.
// Clustering by touch is tempting for multi-sprite characters, but it also
// merges unrelated sprites (a tree and Mickey, an enemy and a platform) and
// bakes their pixels into shared tiles. Per-sprite objects are safer: they keep
// each hardware sprite coherent and avoid cross-contamination.
func groupSprites(sprites []visibleSprite, cluster bool) [][]visibleSprite {
	if !cluster {
		groups := make([][]visibleSprite, len(sprites))
		for i, sprite := range sprites {
			groups[i] = []visibleSprite{sprite}
		}
		return groups
	}
	parents := make([]int, len(sprites))
	for i := range parents {
		parents[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parents[i] != i {
			parents[i] = find(parents[i])
		}
		return parents[i]
	}
	for i := range sprites {
		for j := i + 1; j < len(sprites); j++ {
			if spritesTouch(sprites[i], sprites[j]) {
				a, b := find(i), find(j)
				if a != b {
					parents[b] = a
				}
			}
		}
	}
	byRoot := make(map[int][]visibleSprite)
	for i, sprite := range sprites {
		root := find(i)
		byRoot[root] = append(byRoot[root], sprite)
	}
	groups := make([][]visibleSprite, 0, len(byRoot))
	for _, group := range byRoot {
		groups = append(groups, group)
	}
	return groups
}

func makeSpriteObjects(sprites []visibleSprite, frame int, tiles []machine.TileManifest, tileImages map[string]*image.RGBA, objects map[string]*objectAccumulator, cluster bool) {
	for _, group := range groupSprites(sprites, cluster) {
		minX, minY := group[0].x, group[0].y
		maxX, maxY := minX+group[0].width, minY+group[0].height
		var placements []machine.TileObjectPlacement
		for _, sprite := range group {
			if sprite.x < minX {
				minX = sprite.x
			}
			if sprite.y < minY {
				minY = sprite.y
			}
			if sprite.x+sprite.width > maxX {
				maxX = sprite.x + sprite.width
			}
			if sprite.y+sprite.height > maxY {
				maxY = sprite.y + sprite.height
			}
			for _, part := range sprite.parts {
				placements = append(placements, machine.TileObjectPlacement{
					Index: part.Index,
					X:     sprite.x + part.X,
					Y:     sprite.y + part.Y,
				})
			}
		}
		var signatureParts []string
		for i := range placements {
			placements[i].X -= minX
			placements[i].Y -= minY
			signatureParts = append(signatureParts, fmt.Sprintf("%s@%d,%d", tiles[placements[i].Index].Hash, placements[i].X, placements[i].Y))
		}
		sort.Strings(signatureParts)
		h := fnv.New64a()
		for _, part := range signatureParts {
			_, _ = h.Write([]byte(part + ";"))
		}
		id := fmt.Sprintf("sprite_%016x", h.Sum64())
		acc := objects[id]
		if acc == nil {
			preview := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
			for _, placement := range placements {
				hash := tiles[placement.Index].Hash
				tile := tileImages[hash]
				if tile == nil {
					continue
				}
				draw.Draw(preview, image.Rect(placement.X, placement.Y, placement.X+8, placement.Y+8), tile, image.Point{}, draw.Over)
			}
			acc = &objectAccumulator{
				manifest: machine.TileObjectManifest{
					ID: id, Kind: "sprite_cluster", Label: "unlabeled",
					Preview:    filepath.ToSlash(filepath.Join("objects", id+".png")),
					FirstFrame: frame, Tiles: placements,
				},
				preview: preview,
			}
			objects[id] = acc
		}
		acc.manifest.FramesSeen++
	}
}

// makeBackgroundObjects groups the visible background into square blocks of
// tiles. AI image-to-image works much better on a coherent 64x64 or 128x128
// block than on isolated 8x8 tiles, and repeated blocks are deduplicated by
// their tile signature.
func makeBackgroundObjects(tileMap [][]int, frame int, tiles []machine.TileManifest, tileImages map[string]*image.RGBA, objects map[string]*objectAccumulator, group, protectedRows int) {
	if group < 2 || len(tileMap) == 0 {
		return
	}
	rows, cols := len(tileMap), len(tileMap[0])
	if protectedRows < 0 {
		protectedRows = 0
	}
	for by := protectedRows; by+group <= rows; by += group {
		for bx := 0; bx+group <= cols; bx += group {
			var placements []machine.TileObjectPlacement
			var signatureParts []string
			for y := 0; y < group; y++ {
				for x := 0; x < group; x++ {
					index := tileMap[by+y][bx+x]
					if index < 0 || index >= len(tiles) {
						continue
					}
					placements = append(placements, machine.TileObjectPlacement{
						Index: index,
						X:     x * 8,
						Y:     y * 8,
					})
					signatureParts = append(signatureParts, fmt.Sprintf("%s@%d,%d", tiles[index].Hash, x*8, y*8))
				}
			}
			// Ignore blocks that are mostly empty (screen edges, cutscenes,
			// partial scrolling) so they do not pollute the AI workspace.
			if len(placements) < (group*group)/3 {
				continue
			}
			sort.Strings(signatureParts)
			h := fnv.New64a()
			for _, part := range signatureParts {
				_, _ = h.Write([]byte(part + ";"))
			}
			id := fmt.Sprintf("bg_%016x", h.Sum64())
			acc := objects[id]
			if acc == nil {
				preview := image.NewRGBA(image.Rect(0, 0, group*8, group*8))
				for _, placement := range placements {
					tile := tileImages[tiles[placement.Index].Hash]
					if tile == nil {
						continue
					}
					draw.Draw(preview, image.Rect(placement.X, placement.Y, placement.X+8, placement.Y+8), tile, image.Point{}, draw.Over)
				}
				acc = &objectAccumulator{
					manifest: machine.TileObjectManifest{
						ID: id, Kind: "background_object", Label: "background",
						Preview:    filepath.ToSlash(filepath.Join("objects", id+".png")),
						FirstFrame: frame, Tiles: placements,
					},
					preview: preview,
				}
				objects[id] = acc
			}
			acc.manifest.FramesSeen++
		}
	}
}

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

// captureVisibleTiles añade los tiles de fondo y sprites visibles en el frame
// actual. Se usa para las pantallas estáticas de título y menú, que quedan
// fuera del recorrido normal de niveles.
func captureVisibleTiles(v *vdp.VDP, addTile func(tileData, cram []byte, isSprite bool) (int, string)) {
	bgCram0 := v.CRAM[0:16]
	bgCram1 := v.CRAM[16:32]
	sprCram := v.CRAM[16:32]
	if isPaletteValid(bgCram0) || isPaletteValid(bgCram1) {
		nameBase := int(v.Reg[2]&0x0E) << 10
		for row := 0; row < 28; row++ {
			for col := 0; col < 32; col++ {
				off := (nameBase + row*64 + col*2) & 0x3FFF
				entry := uint16(v.VRAM[(off+1)&0x3FFF])<<8 | uint16(v.VRAM[off])
				tile := int(entry & 0x1FF)
				pal := entry&0x800 != 0
				pat := (tile * 32) & 0x3FFF
				cram := bgCram0
				if pal {
					cram = bgCram1
				}
				if isPaletteValid(cram) {
					addTile(v.VRAM[pat:pat+32], cram, false)
				}
			}
		}
	}
	if !isPaletteValid(sprCram) {
		return
	}
	satBase := int(v.Reg[5]&0x7E) << 7
	patBase := int(v.Reg[6]&0x04) << 11
	tall := v.Reg[1]&0x02 != 0
	for i := 0; i < 64; i++ {
		rawY := int(v.VRAM[(satBase+i)&0x3FFF])
		if rawY == 0 {
			continue
		}
		if rawY == 0xD0 {
			break
		}
		y := (rawY + 1) & 0xFF
		if y >= vdp.ScreenH || (tall && y+16 > vdp.ScreenH) {
			continue
		}
		tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
		parts := 1
		if tall {
			parts = 2
			tile &^= 1
		}
		for p := 0; p < parts; p++ {
			pat := (patBase + (tile+p)*32) & 0x3FFF
			addTile(v.VRAM[pat:pat+32], sprCram, true)
		}
	}
}

func isBlankTile(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func loadHumanObjectMetadata(path string) (map[string]string, []machine.TileObjectManifest) {
	labels := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || data[0] == '[' {
		return labels, nil
	}
	var previous machine.TileManifestDocument
	if err := json.Unmarshal(data, &previous); err != nil {
		return labels, nil
	}
	var custom []machine.TileObjectManifest
	for _, object := range previous.Objects {
		if object.Label != "" {
			labels[object.ID] = object.Label
		}
		if object.Kind == "background_object" {
			custom = append(custom, object)
		}
	}
	return labels, custom
}

func main() {
	frames := flag.Int("frames", extractionFrames, "número de frames usados para descubrir tiles y objetos")
	bot := flag.String("bot", "default", "perfil de controles: default o alex-kidd")
	sonic := flag.Bool("sonic", false, "recorre los 18 niveles de Sonic 1 usando el selector de nivel de RAM")
	castle := flag.Bool("castle", false, "recorre los 6 mundos de Castle of Illusion entrando por las puertas del castillo")
	spriteClusters := flag.Bool("sprite-clusters", false, "agrupa sprites que se tocan (puede mezclar objetos; desactivado por defecto)")
	bgGroup := flag.Int("bg-group", 0, "tamaño en tiles de los bloques de fondo para remasterizar (0 = desactivado, 8 = 64x64 px)")
	bgProtect := flag.Int("bg-protect", 0, "filas superiores de la name table que no se agrupan (HUD)")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Println("Uso: dump_assets [-frames N] [-bot default|alex-kidd] [-sonic] [-castle] [-bg-group 8] [-bg-protect 2] <rom.sms>")
		return
	}
	if *frames < 1 {
		fmt.Println("El número de frames debe ser mayor que cero.")
		return
	}
	if *bot != "default" && *bot != "alex-kidd" {
		fmt.Printf("Bot desconocido %q; opciones: default, alex-kidd\n", *bot)
		return
	}
	romPath := flag.Arg(0)
	data, err := os.ReadFile(romPath)
	if err != nil {
		fmt.Printf("Error leyendo ROM: %v\n", err)
		return
	}
	staticAlexKidd := len(data) == 128*1024 && crc32.ChecksumIEEE(data) == alexKiddV1CRC32

	baseName := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	outDir := filepath.Join(".", "assets", baseName, "original")
	hdDir := filepath.Join(".", "assets", baseName, "hd")
	labels, customObjects := loadHumanObjectMetadata(filepath.Join(outDir, "manifest.json"))
	tileDir := filepath.Join(outDir, "tiles")
	objectDir := filepath.Join(outDir, "objects")
	sceneDir := filepath.Join(outDir, "scenes")
	for _, dir := range []string{tileDir, objectDir, sceneDir, hdDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creando %s: %v\n", dir, err)
			return
		}
	}

	fmt.Printf("🚀 Ejecutando volcado offline de alta fidelidad para '%s'...\n", baseName)
	romTileOffsets := indexROMTiles(data)

	sessions := []extractionSession{{level: -1, frames: *frames}}
	if *sonic {
		if len(data) == 0 || baseName == "" {
			fmt.Println("No se pudo preparar el recorrido de niveles de Sonic.")
			return
		}
		sessions = sonicSessions(*frames)
		fmt.Printf("🎮 Sonic 1: recorriendo %d niveles (selector RAM $%04X).\n", len(sessions), sonicLevelRAM)
	}
	if *castle {
		sessions = castleSessions(*frames)
		fmt.Printf("🏰 Castle of Illusion: recorriendo %d mundos (puerta RAM $%04X).\n", len(sessions), castleDoorEntityRAM)
	}

	uniqueTiles := make(map[string]*image.RGBA)
	var manifest []machine.TileManifest
	tileIndexByHash := make(map[string]int)
	objects := make(map[string]*objectAccumulator)
	var scenes []machine.TileSceneManifest

	addTile := func(tileData []byte, cram []byte, isSprite bool) (int, string) {
		if isBlankTile(tileData) {
			return -1, ""
		}
		h := machine.TileHash(tileData, cram, isSprite)
		if index, exists := tileIndexByHash[h]; exists {
			return index, h
		}
		index := len(manifest)
		uniqueTiles[h] = decodeTile(tileData, cram, isSprite)
		tileIndexByHash[h] = index
		romOffsets := romTileOffsets[tileKey(tileData)]
		if len(romOffsets) > 8 {
			romOffsets = romOffsets[:8]
		}
		manifest = append(manifest, machine.TileManifest{
			Hash: h, Index: index, IsSprite: isSprite, ROMOffsets: append([]int(nil), romOffsets...),
		})
		return index, h
	}

	for _, session := range sessions {
		m := machine.New(data)
		totalFrames := session.frames
		if totalFrames == 0 {
			totalFrames = *frames
		}
		for f := 0; f < totalFrames; f++ {
			globalFrame := f
			level := session.level
			isBoot := *castle && f < session.bootFrames
			if *sonic {
				globalFrame = session.level*10000 + f
				if f < 700 {
					m.CPU.RAM[(sonicLevelRAM-0xC000)&0x1FFF] = byte(session.level)
				}
				m.CPU.RAM[(sonicLivesRAM-0xC000)&0x1FFF] = 9
			}
			if *castle {
				// La puerta se recrea al cargar el recibidor; mantenemos el
				// índice del mundo deseado hasta que arranque el nivel.
				if f >= 3380 && f < 3900 {
					m.CPU.RAM[(castleDoorEntityRAM-0xC000)&0x1FFF] = byte(0x80 | level)
				}
				if f >= 3400 {
					m.CPU.RAM[(0xC0BD-0xC000)&0x1FFF] = 3 // energía infinita
					m.CPU.RAM[(0xC0C8-0xC000)&0x1FFF] = 3 // vidas infinitas
				}
				m.CPU.Pad1 = castlePad(f, *bot, session.bootFrames)
			} else {
				m.CPU.Pad1 = inputForFrame(f, *bot)
			}
			m.RunFrame()

			if isBoot {
				continue
			}
			if *castle {
				globalFrame = session.level*10000 + (f - session.bootFrames)
			}

			// A completed level advances g_level; stop this session so the
			// next one starts cleanly instead of capturing the next zone twice.
			if *sonic && f > 750 && int(m.CPU.RAM[(sonicLevelRAM-0xC000)&0x1FFF]) != session.level {
				fmt.Printf("🏁 %s completado en el frame %d.\n", session.name, f)
				break
			}

			if f%2 == 0 {
				v := m.VDP
				bgCram0 := v.CRAM[0:16]
				bgCram1 := v.CRAM[16:32]
				sprCram := v.CRAM[16:32]
				tileMap := make([][]int, 28)
				for row := range tileMap {
					tileMap[row] = make([]int, 32)
					for col := range tileMap[row] {
						tileMap[row][col] = -1
					}
				}

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
								if index, _ := addTile(tileData, cram, false); index >= 0 {
									tileMap[row][col] = index
								}
							}
						}
					}
				}

				// Capturar sprites visibles en pantalla
				if isPaletteValid(sprCram) {
					satBase := int(v.Reg[5]&0x7E) << 7
					patBase := int(v.Reg[6]&0x04) << 11
					tall := v.Reg[1]&0x02 != 0
					var visible []visibleSprite

					for i := 0; i < 64; i++ {
						rawY := int(v.VRAM[(satBase+i)&0x3FFF])
						if rawY == 0 {
							continue // La SAT vacía inicializada a cero no describe un sprite activo.
						}
						y := rawY
						if y == 0xD0 {
							break
						}
						x := int(v.VRAM[(satBase+128+i*2)&0x3FFF])
						y = (y + 1) & 0xFF
						if y >= vdp.ScreenH || (tall && y+16 > vdp.ScreenH) {
							continue
						}
						tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
						parts := 1
						if tall {
							parts = 2
							tile &^= 1
						}
						sprite := visibleSprite{x: x, y: y, width: 8, height: 8}
						if tall {
							sprite.height = 16
						}
						for p := 0; p < parts; p++ {
							pat := (patBase + (tile+p)*32) & 0x3FFF
							tileData := v.VRAM[pat : pat+32]
							index, _ := addTile(tileData, sprCram, true)
							if index < 0 {
								continue
							}
							sprite.parts = append(sprite.parts, machine.TileObjectPlacement{Index: index, Y: p * 8})
						}
						if len(sprite.parts) > 0 {
							visible = append(visible, sprite)
						}
					}
					makeSpriteObjects(visible, globalFrame, manifest, uniqueTiles, objects, *spriteClusters)
				}

				if *bgGroup >= 2 && f%30 == 0 {
					makeBackgroundObjects(tileMap, globalFrame, manifest, uniqueTiles, objects, *bgGroup, *bgProtect)
				}

				if !staticAlexKidd && f%120 == 0 {
					imageName := fmt.Sprintf("frame_%05d.png", globalFrame)
					scene := machine.TileSceneManifest{
						Frame: globalFrame, Image: filepath.ToSlash(filepath.Join("scenes", imageName)),
						ScrollX: int(v.Reg[8]), ScrollY: int(v.Reg[9]), TileMap: tileMap,
					}
					if *sonic || *castle {
						scene.Level = level
						scene.LevelName = session.name
					}
					scenes = append(scenes, scene)
					sceneImage := image.NewRGBA(image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH))
					copy(sceneImage.Pix, v.Frame[:])
					if file, err := os.Create(filepath.Join(sceneDir, imageName)); err == nil {
						_ = png.Encode(file, sceneImage)
						_ = file.Close()
					}
				}
			}
		}
	}
	// El título y el menú de Castle viven en los boot frames que las sesiones
	// saltan. Se capturan al final para que sus tiles se añadan detrás de los
	// del gameplay y no desplacen ningún índice existente.
	if *castle {
		windows := [][2]int{{860, 990}, {1070, 1190}}
		titles := machine.New(data)
		before := len(manifest)
		for f := 0; f <= windows[len(windows)-1][1]; f++ {
			titles.CPU.Pad1 = castlePad(f, *bot, castleBootFrames)
			titles.RunFrame()
			capture := false
			for _, w := range windows {
				if f >= w[0] && f <= w[1] {
					capture = true
					break
				}
			}
			if !capture || f%10 != 0 {
				continue
			}
			captureVisibleTiles(titles.VDP, addTile)
		}
		fmt.Printf("🎬 Título y menú: %d tiles nuevos.\n", len(manifest)-before)
	}

	if staticAlexKidd {
		for level := 1; level <= len(alexKiddLevelNames); level++ {
			if err := addStaticTileSets(data, level, addTile, uniqueTiles, objects); err != nil {
				fmt.Printf("Error decodificando gráficos estáticos del nivel %d: %v\n", level, err)
				os.Exit(1)
			}
		}
		staticScenes, count, err := alexKiddLayoutScenes(data, sceneDir)
		if err != nil {
			fmt.Printf("Error decodificando mapas estáticos de Alex Kidd: %v\n", err)
			os.Exit(1)
		}
		scenes = staticScenes
		fmt.Printf("🧩 Tiles estáticos: %d tras decodificar los gráficos de los 17 niveles.\n", len(manifest))
		fmt.Printf("🗺️ Mapas estáticos: %d bloques de pantalla encontrados en los 17 niveles.\n", count)
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

	for _, item := range manifest {
		prefix := "bg"
		if item.IsSprite {
			prefix = "spr"
		}
		file, err := os.Create(filepath.Join(tileDir, fmt.Sprintf("%s_%04d.png", prefix, item.Index)))
		if err != nil {
			continue
		}
		_ = png.Encode(file, uniqueTiles[item.Hash])
		_ = file.Close()
	}

	ids := make([]string, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	objectManifest := make([]machine.TileObjectManifest, 0, len(ids))
	for _, id := range ids {
		object := objects[id]
		if label := labels[id]; label != "" {
			object.manifest.Label = label
		}
		file, err := os.Create(filepath.Join(objectDir, id+".png"))
		if err == nil {
			_ = png.Encode(file, object.preview)
			_ = file.Close()
		}
		objectManifest = append(objectManifest, object.manifest)
	}
	for _, object := range customObjects {
		if _, exists := objects[object.ID]; !exists {
			objectManifest = append(objectManifest, object)
		}
	}
	document := machine.TileManifestDocument{
		Version: 2, ROM: filepath.Base(romPath), FramesAnalyzed: *frames,
		Tiles: manifest, Objects: objectManifest, Scenes: scenes,
	}
	if err := saveJSON(filepath.Join(outDir, "manifest.json"), document); err != nil {
		fmt.Printf("Error guardando manifest: %v\n", err)
		return
	}
	if err := saveJSON(filepath.Join(hdDir, "manifest.json"), document); err != nil {
		fmt.Printf("Error guardando manifest HD: %v\n", err)
		return
	}

	romBacked := 0
	for _, tile := range manifest {
		if len(tile.ROMOffsets) > 0 {
			romBacked++
		}
	}
	backgroundObjects := 0
	for _, object := range objectManifest {
		if object.Kind == "background_object" {
			backgroundObjects++
		}
	}
	fmt.Printf("✅ VOLCADO COMPLETADO: %d tiles (%d encontrados sin comprimir en ROM), %d composites (%d de fondo) y %d mapas de escena\n",
		len(manifest), romBacked, len(objectManifest), backgroundObjects, len(scenes))
}

func saveJSON(path string, data interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
