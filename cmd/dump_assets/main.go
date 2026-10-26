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

func makeSpriteObjects(sprites []visibleSprite, frame int, tiles []machine.TileManifest, tileImages map[string]*image.RGBA, objects map[string]*objectAccumulator) {
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

	groups := make(map[int][]visibleSprite)
	for i, sprite := range sprites {
		root := find(i)
		groups[root] = append(groups[root], sprite)
	}

	for _, group := range groups {
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
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Println("Uso: dump_assets [-frames N] [-bot default|alex-kidd] <rom.sms>")
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
	m := machine.New(data)
	romTileOffsets := indexROMTiles(data)

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

	for f := 0; f < *frames; f++ {
		m.CPU.Pad1 = inputForFrame(f, *bot)
		m.RunFrame()

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
				makeSpriteObjects(visible, f, manifest, uniqueTiles, objects)
			}

			if !staticAlexKidd && f%120 == 0 {
				imageName := fmt.Sprintf("frame_%05d.png", f)
				scenes = append(scenes, machine.TileSceneManifest{
					Frame: f, Image: filepath.ToSlash(filepath.Join("scenes", imageName)),
					ScrollX: int(v.Reg[8]), ScrollY: int(v.Reg[9]), TileMap: tileMap,
				})
				sceneImage := image.NewRGBA(image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH))
				copy(sceneImage.Pix, v.Frame[:])
				if file, err := os.Create(filepath.Join(sceneDir, imageName)); err == nil {
					_ = png.Encode(file, sceneImage)
					_ = file.Close()
				}
			}
		}
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
	fmt.Printf("✅ VOLCADO COMPLETADO: %d tiles (%d encontrados sin comprimir en ROM), %d composites y %d mapas de escena\n", len(manifest), romBacked, len(objectManifest), len(scenes))
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
