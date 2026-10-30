// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package machine

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

type TileManifest struct {
	Hash       string `json:"hash"`
	Index      int    `json:"index"`
	IsSprite   bool   `json:"is_sprite"`
	ROMOffsets []int  `json:"rom_offsets,omitempty"`
}

// TileManifestDocument keeps runtime tile lookup data alongside the human-readable
// object catalog produced by the asset extractor.
type TileManifestDocument struct {
	Version        int                  `json:"version"`
	ROM            string               `json:"rom"`
	FramesAnalyzed int                  `json:"frames_analyzed"`
	Tiles          []TileManifest       `json:"tiles"`
	Objects        []TileObjectManifest `json:"objects,omitempty"`
	Scenes         []TileSceneManifest  `json:"scenes,omitempty"`
}

type TileObjectManifest struct {
	ID         string                `json:"id"`
	Kind       string                `json:"kind"`
	Label      string                `json:"label"`
	Preview    string                `json:"preview"`
	FramesSeen int                   `json:"frames_seen"`
	FirstFrame int                   `json:"first_frame"`
	Tiles      []TileObjectPlacement `json:"tiles"`
}

type TileObjectPlacement struct {
	Index int `json:"index"`
	X     int `json:"x"`
	Y     int `json:"y"`
}

type TileSceneManifest struct {
	Frame       int     `json:"frame,omitempty"`
	Image       string  `json:"image"`
	ScrollX     int     `json:"scroll_x,omitempty"`
	ScrollY     int     `json:"scroll_y,omitempty"`
	TileMap     [][]int `json:"tile_map,omitempty"`
	Level       int     `json:"level,omitempty"`
	LevelName   string  `json:"level_name,omitempty"`
	Screen      int     `json:"screen,omitempty"`
	ROMOffset   int     `json:"rom_offset,omitempty"`
	MetatileMap [][]int `json:"metatile_map,omitempty"`
}

// HDAtlasPage describes one packed texture page. Large packs are split into
// pages so they stay below the GPU's maximum texture size.
type HDAtlasPage struct {
	File   string `json:"file"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// HDAtlasTile locates one remastered 8x8 tile inside a page.
type HDAtlasTile struct {
	Hash     string `json:"hash"`
	Page     int    `json:"page"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	W        int    `json:"w"`
	H        int    `json:"h"`
	IsSprite bool   `json:"is_sprite"`
}

// HDPackDocument is the v2 HD pack format. It is intentionally separate from
// TileManifestDocument so the runtime only parses what it needs.
type HDPackDocument struct {
	Version     int                  `json:"version"`
	ROM         string               `json:"rom"`
	Scale       int                  `json:"scale"`
	TileSize    int                  `json:"tile_size"`
	Pages       []HDAtlasPage        `json:"pages"`
	HeightPages []HDAtlasPage        `json:"height_pages,omitempty"`
	Tiles       []HDAtlasTile        `json:"tiles"`
	Objects     []TileObjectManifest `json:"objects,omitempty"`
	Scenes      []TileSceneManifest  `json:"scenes,omitempty"`
}

// HDOverlayPattern describe una celda del patron de un objeto: su posicion en
// celdas de 8x8 y el hash del tile que debe haber en el name table.
type HDOverlayPattern struct {
	DX   int    `json:"dx"`
	DY   int    `json:"dy"`
	Hash string `json:"hash"`
}

// HDOverlayEntry es un objeto que se dibuja como una sola imagen en vez de
// tile a tile. Sirve para bloques cuyos tiles se reutilizan y no pueden
// conservar el detalle de una imagen completa.
type HDOverlayEntry struct {
	ID      string             `json:"id"`
	Image   string             `json:"image"`
	Width   int                `json:"width"`
	Height  int                `json:"height"`
	Pattern []HDOverlayPattern `json:"pattern"`
}

type HDOverlayDocument struct {
	Version  int              `json:"version"`
	Scale    int              `json:"scale"`
	Overlays []HDOverlayEntry `json:"overlays"`
}

// HDOverlay es la version cargada en memoria de HDOverlayEntry.
type HDOverlay struct {
	ID      string
	Image   *ebiten.Image
	Width   int
	Height  int
	Pattern []HDOverlayPattern
}

// HDPalette es el mapa de color de los 64 registros CRAM ya graduados. El
// fondo del VDP se pinta con un registro, no con un tile, asi que necesita su
// propia traduccion para que el cielo siga la paleta del pack.
type HDPalette struct {
	Grade  string            `json:"grade"`
	Colors map[string][3]int `json:"colors"`
}

type HDRenderer struct {
	Active bool
	Sheet  *ebiten.Image

	// Palette traduce el color del fondo CRT a la paleta del pack. Puede ser
	// nil: entonces el fondo se queda con el color de la consola.
	Palette map[color.RGBA]color.RGBA

	// HeightSheet es el atlas de altura en packs v1 (mismo layout que Sheet).
	HeightSheet *ebiten.Image
	HeightMap   map[string]*ebiten.Image

	// En packs v2 las alturas viven en sus propias paginas y se cargan bajo
	// demanda igual que el color.
	HeightPagePaths  []string
	HeightPages      []*ebiten.Image
	HeightPageLoaded []bool
	HasHeight        bool

	// Pages are loaded lazily from PagePaths. This keeps an 8x pack from
	// reserving hundreds of MiB before the first scene is drawn.
	Pages      []*ebiten.Image
	PagePaths  []string
	PageSizes  []image.Point
	PageLoaded []bool
	TileRefs   map[string]HDAtlasTile

	TileMap   map[string]*ebiten.Image
	ShadowMap map[string]*ebiten.Image

	// PriorityMap guarda la version de un tile de fondo con el indice 0
	// transparente. Solo la usan las baldosas prioritarias que deben dejar ver
	// los sprites detras de sus huecos, como hace el VDP nativo.
	PriorityMap  map[string]*ebiten.Image
	PriorityMask map[string]*ebiten.Image

	// Overlays son objetos que se dibujan como una imagen completa sobre el
	// name table cuando su patron de tiles coincide.
	Overlays []HDOverlay

	Scale int
}

var HD *HDRenderer

func TileHash(tileData []byte, cram []byte, isSprite bool) string {
	h := fnv.New64a()
	h.Write(tileData)
	h.Write(cram)
	if isSprite {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// GetPriorityTile devuelve el tile HD con sus pixeles de indice 0 recortados.
// El VDP nativo pinta el fondo prioritario antes que los sprites y los deja
// pasar por los pixeles transparentes; el pack HD guarda esos huecos opacos,
// asi que esta variante se dibuja encima de los sprites para reproducir el
// mismo recorte.
func (hd *HDRenderer) GetPriorityTile(tileData, cram []byte, img *ebiten.Image) *ebiten.Image {
	if hd == nil || img == nil || len(tileData) < 32 {
		return img
	}
	if hd.PriorityMap == nil {
		hd.PriorityMap = make(map[string]*ebiten.Image)
	}
	key := TileHash(tileData, cram, false)
	if cached, ok := hd.PriorityMap[key]; ok {
		return cached
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	masked := ebiten.NewImage(width, height)
	masked.DrawImage(img, nil)

	op := &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(float64(width)/8.0, float64(height)/8.0)
	masked.DrawImage(hd.priorityMask(tileData), op)
	hd.PriorityMap[key] = masked
	return masked
}

// priorityMask crea la silueta de una baldosa: opaca donde el indice de color
// no es 0. Se cachea por patron de VRAM porque no depende de la paleta.
func (hd *HDRenderer) priorityMask(tileData []byte) *ebiten.Image {
	if hd.PriorityMask == nil {
		hd.PriorityMask = make(map[string]*ebiten.Image)
	}
	key := string(tileData)
	if cached, ok := hd.PriorityMask[key]; ok {
		return cached
	}
	mask := ebiten.NewImage(8, 8)
	mask.WritePixels(priorityMaskPixels(tileData))
	hd.PriorityMask[key] = mask
	return mask
}

// priorityMaskPixels construye la silueta RGBA de una baldosa: blanca y opaca
// donde el indice de color no es 0, y transparente en los huecos.
func priorityMaskPixels(tileData []byte) []byte {
	buf := make([]byte, 8*8*4)
	for i := 0; i < 8*8; i++ {
		buf[i*4] = 255
		buf[i*4+1] = 255
		buf[i*4+2] = 255
		buf[i*4+3] = 255
	}
	for y := 0; y < 8 && y*4+3 < len(tileData); y++ {
		b0, b1, b2, b3 := tileData[y*4], tileData[y*4+1], tileData[y*4+2], tileData[y*4+3]
		for x := 0; x < 8; x++ {
			bit := uint(7 - x)
			c := ((b0 >> bit) & 1) | (((b1 >> bit) & 1) << 1) | (((b2 >> bit) & 1) << 2) | (((b3 >> bit) & 1) << 3)
			if c == 0 {
				buf[(y*8+x)*4+3] = 0
			}
		}
	}
	return buf
}

func decodeColorVDP(c byte) color.RGBA {
	r := (c & 3) * 85
	g := ((c >> 2) & 3) * 85
	b := ((c >> 4) & 3) * 85
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func (hd *HDRenderer) generateFallbackTile(tileData []byte, cram []byte, isSprite bool, scale int) *ebiten.Image {
	if scale < 1 {
		scale = 1
	}
	tileSize := 8 * scale
	img := image.NewRGBA(image.Rect(0, 0, tileSize, tileSize))

	pal := make([]color.RGBA, 16)
	for i := 0; i < 16 && i < len(cram); i++ {
		pal[i] = decodeColorVDP(cram[i])
	}

	for y := 0; y < 8; y++ {
		var b0, b1, b2, b3 byte
		if y*4+3 < len(tileData) {
			b0 = tileData[y*4]
			b1 = tileData[y*4+1]
			b2 = tileData[y*4+2]
			b3 = tileData[y*4+3]
		}
		for x := 0; x < 8; x++ {
			bit := 7 - x
			cIdx := ((b0 >> bit) & 1) | (((b1 >> bit) & 1) << 1) | (((b2 >> bit) & 1) << 2) | (((b3 >> bit) & 1) << 3)
			var col color.RGBA
			if isSprite && cIdx == 0 {
				col = color.RGBA{0, 0, 0, 0}
			} else {
				col = hd.GradeColor(pal[cIdx&0x0F])
				col.A = 255
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(x*scale+dx, y*scale+dy, col)
				}
			}
		}
	}
	return ebiten.NewImageFromImage(img)
}

func loadHDPalette(hdDir string) map[color.RGBA]color.RGBA {
	data, err := os.ReadFile(filepath.Join(hdDir, "palette.json"))
	if err != nil {
		return nil
	}
	var doc HDPalette
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	palette := make(map[color.RGBA]color.RGBA, len(doc.Colors))
	for key, rgb := range doc.Colors {
		var r, g, b int
		if _, err := fmt.Sscanf(key, "%d,%d,%d", &r, &g, &b); err != nil {
			continue
		}
		palette[color.RGBA{uint8(r), uint8(g), uint8(b), 255}] = color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
	}
	if len(palette) == 0 {
		return nil
	}
	return palette
}

// GradeColor traduce un color de la consola al del pack. Sin paleta devuelve
// el mismo color, de modo que los packs antiguos siguen funcionando.
func (hd *HDRenderer) GradeColor(c color.RGBA) color.RGBA {
	if hd == nil || hd.Palette == nil {
		return c
	}
	if mapped, ok := hd.Palette[c]; ok {
		return mapped
	}
	return c
}

func InitHD(romPath string) {
	HD = &HDRenderer{Active: false}

	baseName := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	hdDir := filepath.Join(".", "assets", baseName, "hd")

	// v2 packs are page-based and can be larger than one texture. Prefer them
	// when present; v1 packs keep loading through the legacy path.
	if data, err := os.ReadFile(filepath.Join(hdDir, "pack.json")); err == nil {
		if loadHDPackV2(hdDir, data) {
			return
		}
	}
	loadHDLegacy(hdDir)
}

func loadHDPackV2(hdDir string, data []byte) bool {
	var doc HDPackDocument
	if err := json.Unmarshal(data, &doc); err != nil || doc.Version < 2 || len(doc.Pages) == 0 {
		return false
	}
	maxSize := ebiten.MaxImageSize()
	pagePaths := make([]string, len(doc.Pages))
	pageSizes := make([]image.Point, len(doc.Pages))
	for i, page := range doc.Pages {
		if page.Width <= 0 || page.Height <= 0 {
			return false
		}
		if maxSize > 0 && (page.Width > maxSize || page.Height > maxSize) {
			fmt.Printf("\033[1;33m⚠️ GOSMS HD:\033[0m página %s (%dx%d) supera el límite de textura %d; usando pack nativo.\n",
				page.File, page.Width, page.Height, maxSize)
			return false
		}
		pagePaths[i] = filepath.Join(hdDir, page.File)
		pageSizes[i] = image.Pt(page.Width, page.Height)
	}

	heightPaths := make([]string, len(doc.HeightPages))
	for i, page := range doc.HeightPages {
		heightPaths[i] = filepath.Join(hdDir, page.File)
	}

	scale := doc.Scale
	if scale < 1 && len(doc.Tiles) > 0 {
		scale = doc.Tiles[0].W / 8
	}
	if scale < 1 {
		scale = 4
	}

	hd := &HDRenderer{
		Active:     true,
		Pages:      make([]*ebiten.Image, len(doc.Pages)),
		PagePaths:  pagePaths,
		PageSizes:  pageSizes,
		PageLoaded: make([]bool, len(doc.Pages)),
		TileRefs:   make(map[string]HDAtlasTile),
		TileMap:    make(map[string]*ebiten.Image),
		HeightMap:  make(map[string]*ebiten.Image),
		ShadowMap:  make(map[string]*ebiten.Image),
		Palette:    loadHDPalette(hdDir),
		Scale:      scale,
	}
	if len(heightPaths) == len(doc.Pages) {
		hd.HeightPagePaths = heightPaths
		hd.HeightPages = make([]*ebiten.Image, len(heightPaths))
		hd.HeightPageLoaded = make([]bool, len(heightPaths))
		hd.HasHeight = true
	}
	mapped := 0
	for _, tile := range doc.Tiles {
		if tile.Page < 0 || tile.Page >= len(pageSizes) || tile.W <= 0 || tile.H <= 0 {
			continue
		}
		size := pageSizes[tile.Page]
		if tile.X < 0 || tile.Y < 0 || tile.X+tile.W > size.X || tile.Y+tile.H > size.Y {
			continue
		}
		hd.TileRefs[tile.Hash] = tile
		mapped++
	}
	if mapped == 0 {
		return false
	}
	HD = hd
	fmt.Printf("\033[1;32m🌟 GOSMS HD ENGINE:\033[0m Pack v2 cargado -> %d texturas en %d páginas (Escala %dx, carga bajo demanda)\n",
		mapped, len(hd.Pages), hd.Scale)
	return true
}

// LoadPage decodes one atlas page on demand. It returns false when the page is
// invalid, missing, or larger than the current backend limit.
func (hd *HDRenderer) LoadPage(index int) bool {
	if hd == nil || index < 0 || index >= len(hd.PagePaths) {
		return false
	}
	if hd.PageLoaded[index] && hd.Pages[index] != nil {
		return true
	}
	size := hd.PageSizes[index]
	if maxSize := ebiten.MaxImageSize(); maxSize > 0 && (size.X > maxSize || size.Y > maxSize) {
		return false
	}
	f, err := os.Open(hd.PagePaths[index])
	if err != nil {
		return false
	}
	decoded, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil || decoded.Bounds().Dx() != size.X || decoded.Bounds().Dy() != size.Y {
		return false
	}
	hd.Pages[index] = ebiten.NewImageFromImage(decoded)
	hd.PageLoaded[index] = true
	return true
}

func loadHDLegacy(hdDir string) {
	sheetPath := filepath.Join(hdDir, "spritesheet.png")
	manifestPath := filepath.Join(hdDir, "manifest.json")
	if _, err := os.Stat(sheetPath); os.IsNotExist(err) {
		return
	}
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return
	}

	mfData, err := os.ReadFile(manifestPath)
	if err != nil {
		return
	}

	var manifest []TileManifest
	if len(mfData) > 0 && mfData[0] == '[' {
		if err := json.Unmarshal(mfData, &manifest); err != nil {
			return
		}
	} else {
		var document TileManifestDocument
		if err := json.Unmarshal(mfData, &document); err != nil {
			return
		}
		manifest = document.Tiles
	}

	f, err := os.Open(sheetPath)
	if err != nil {
		return
	}
	defer f.Close()

	decodedImg, _, err := image.Decode(f)
	if err != nil {
		return
	}
	if maxSize := ebiten.MaxImageSize(); maxSize > 0 &&
		(decodedImg.Bounds().Dx() > maxSize || decodedImg.Bounds().Dy() > maxSize) {
		fmt.Printf("\033[1;33m⚠️ GOSMS HD:\033[0m spritesheet %dx%d supera el límite de textura %d; usando pack nativo.\n",
			decodedImg.Bounds().Dx(), decodedImg.Bounds().Dy(), maxSize)
		return
	}
	img := ebiten.NewImageFromImage(decodedImg)

	hd := &HDRenderer{
		Active:    true,
		Sheet:     img,
		TileMap:   make(map[string]*ebiten.Image),
		HeightMap: make(map[string]*ebiten.Image),
		ShadowMap: make(map[string]*ebiten.Image),
		Palette:   loadHDPalette(hdDir),
	}
	cols := 32
	scale := img.Bounds().Dx() / (cols * 12)
	if scale < 1 {
		scale = 1
	}
	hd.Scale = scale
	hd.Overlays = loadHDOverlays(hdDir)

	cellSize := 12 * scale
	padding := 2 * scale
	tileSize := 8 * scale
	imgWidth := img.Bounds().Dx()

	for _, m := range manifest {
		gx := (m.Index%cols)*cellSize + padding
		gy := (m.Index/cols)*cellSize + padding
		if gx+tileSize > imgWidth || gy+tileSize > img.Bounds().Dy() {
			continue
		}
		rect := image.Rect(gx, gy, gx+tileSize, gy+tileSize)
		hd.TileMap[m.Hash] = img.SubImage(rect).(*ebiten.Image)
	}

	HD = hd
	fmt.Printf("\033[1;32m🌟 GOSMS HD ENGINE:\033[0m Pack v1 cargado -> %d texturas (Escala %dx)\n", len(hd.TileMap), hd.Scale)
}

// loadHDOverlays carga las imagenes de objeto completo declaradas en
// overlays.json. Si el archivo no existe, el pack funciona igual que antes.
func loadHDOverlays(hdDir string) []HDOverlay {
	data, err := os.ReadFile(filepath.Join(hdDir, "overlays.json"))
	if err != nil {
		return nil
	}
	var doc HDOverlayDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	overlays := make([]HDOverlay, 0, len(doc.Overlays))
	for _, entry := range doc.Overlays {
		if entry.ID == "" || entry.Image == "" || len(entry.Pattern) == 0 {
			continue
		}
		f, err := os.Open(filepath.Join(hdDir, filepath.FromSlash(entry.Image)))
		if err != nil {
			continue
		}
		decoded, _, err := image.Decode(f)
		_ = f.Close()
		if err != nil {
			continue
		}
		overlays = append(overlays, HDOverlay{
			ID:      entry.ID,
			Image:   ebiten.NewImageFromImage(decoded),
			Width:   entry.Width,
			Height:  entry.Height,
			Pattern: entry.Pattern,
		})
	}
	if len(overlays) > 0 {
		fmt.Printf("\033[1;32m🌟 GOSMS HD:\033[0m %d overlay(s) de objeto completo.\n", len(overlays))
	}
	return overlays
}

// LoadHeightPage decodifica una pagina de altura bajo demanda.
func (hd *HDRenderer) LoadHeightPage(index int) bool {
	if hd == nil || !hd.HasHeight || index < 0 || index >= len(hd.HeightPagePaths) {
		return false
	}
	if hd.HeightPageLoaded[index] && hd.HeightPages[index] != nil {
		return true
	}
	f, err := os.Open(hd.HeightPagePaths[index])
	if err != nil {
		return false
	}
	decoded, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil || hd.HeightPages == nil {
		return false
	}
	hd.HeightPages[index] = ebiten.NewImageFromImage(decoded)
	hd.HeightPageLoaded[index] = true
	return true
}

// GetHDHeight devuelve el mapa de altura del tile, o nil si el pack no lo trae.
func (hd *HDRenderer) GetHDHeight(tileData []byte, cram []byte, isSprite bool) *ebiten.Image {
	if hd == nil || !hd.Active || !hd.HasHeight {
		return nil
	}
	key := TileHash(tileData, cram, isSprite)
	if img, ok := hd.HeightMap[key]; ok {
		return img
	}
	ref, ok := hd.TileRefs[key]
	if !ok || !hd.LoadHeightPage(ref.Page) {
		return nil
	}
	page := hd.HeightPages[ref.Page]
	if page == nil {
		return nil
	}
	rect := image.Rect(ref.X, ref.Y, ref.X+ref.W, ref.Y+ref.H)
	if rect.Min.X < 0 || rect.Min.Y < 0 || rect.Max.X > page.Bounds().Dx() || rect.Max.Y > page.Bounds().Dy() {
		return nil
	}
	img := page.SubImage(rect).(*ebiten.Image)
	hd.HeightMap[key] = img
	return img
}

func (hd *HDRenderer) GetHDTile(tileData []byte, cram []byte, isSprite bool) *ebiten.Image {
	if !hd.Active {
		return nil
	}
	hStr := TileHash(tileData, cram, isSprite)
	if img, ok := hd.TileMap[hStr]; ok {
		return img
	}

	if ref, ok := hd.TileRefs[hStr]; ok && hd.LoadPage(ref.Page) {
		page := hd.Pages[ref.Page]
		rect := image.Rect(ref.X, ref.Y, ref.X+ref.W, ref.Y+ref.H)
		if rect.Min.X >= 0 && rect.Min.Y >= 0 && rect.Max.X <= page.Bounds().Dx() && rect.Max.Y <= page.Bounds().Dy() {
			img := page.SubImage(rect).(*ebiten.Image)
			hd.TileMap[hStr] = img
			return img
		}
	}

	// Fallback dinámico: genera y cachea el tile nativo a escala HD si no está remasterizado
	img := hd.generateFallbackTile(tileData, cram, isSprite, hd.Scale)
	hd.TileMap[hStr] = img
	return img
}
