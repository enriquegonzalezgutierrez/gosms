package machine

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

type TileManifest struct {
	Hash       string `json:"hash"`
	Index      int    `json:"index"`
	PaletteIdx int    `json:"palette"`
}

type HDRenderer struct {
	Active  bool
	Sheet   *ebiten.Image
	TileMap map[string]*ebiten.Image
	Scale   int
}

var HD *HDRenderer

func InitHD(romPath string) {
	HD = &HDRenderer{Active: false}

	baseName := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	hdDir := filepath.Join(".", "assets", baseName, "hd")
	sheetPath := filepath.Join(hdDir, "spritesheet.png")
	manifestPath := filepath.Join(hdDir, "manifest.json")

	if _, err := os.Stat(sheetPath); os.IsNotExist(err) {
		fmt.Println("HD: Spritesheet no encontrado en", sheetPath)
		return
	}
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		fmt.Println("HD: Manifest.json no encontrado en", manifestPath)
		return
	}

	mfData, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Println("HD: Error leyendo manifest:", err)
		return
	}

	var manifest []TileManifest
	if err := json.Unmarshal(mfData, &manifest); err != nil {
		fmt.Println("HD: Error parseando manifest:", err)
		return
	}

	img, _, err := ebitenutil.NewImageFromFile(sheetPath)
	if err != nil {
		fmt.Println("HD: Error cargando imagen:", err)
		return
	}

	HD.Sheet = img
	HD.TileMap = make(map[string]*ebiten.Image)

	cols := 32
	scale := img.Bounds().Dx() / (cols * 8)
	if scale < 1 {
		scale = 1
	}
	HD.Scale = scale
	tileSize := 8 * scale
	imgWidth := img.Bounds().Dx()

	for _, m := range manifest {
		gx := (m.Index % cols) * tileSize
		gy := (m.Index / cols) * tileSize
		
		if gx+tileSize > imgWidth || gy+tileSize > img.Bounds().Dy() {
			continue
		}

		rect := image.Rect(gx, gy, gx+tileSize, gy+tileSize)
		subImg := img.SubImage(rect).(*ebiten.Image)
		HD.TileMap[m.Hash] = subImg
	}

	HD.Active = true
	fmt.Printf("🌟 PACK HD CARGADO CON ÉXITO: %d texturas vinculadas (Escala %dx)\n", len(HD.TileMap), HD.Scale)
}

func (hd *HDRenderer) GetHDTile(tileData []byte, palIdx int) *ebiten.Image {
	if !hd.Active {
		return nil
	}
	h := fnv.New64a()
	h.Write(tileData)
	h.Write([]byte{byte(palIdx)})
	hashStr := fmt.Sprintf("%016x", h.Sum64())

	return hd.TileMap[hashStr]
}
