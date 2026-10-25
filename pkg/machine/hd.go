package machine

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

type TileManifest struct {
	Hash     string `json:"hash"`
	Index    int    `json:"index"`
	IsSprite bool   `json:"is_sprite"`
}

type HDRenderer struct {
	Active  bool
	Sheet   *ebiten.Image
	TileMap map[string]*ebiten.Image
	Scale   int
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

func InitHD(romPath string) {
	HD = &HDRenderer{Active: false}

	baseName := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	hdDir := filepath.Join(".", "assets", baseName, "hd")
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
	if err := json.Unmarshal(mfData, &manifest); err != nil {
		return
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
	img := ebiten.NewImageFromImage(decodedImg)

	HD.Sheet = img
	HD.TileMap = make(map[string]*ebiten.Image)

	cols := 32
	scale := img.Bounds().Dx() / (cols * 12)
	if scale < 1 {
		scale = 1
	}
	HD.Scale = scale

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
		HD.TileMap[m.Hash] = img.SubImage(rect).(*ebiten.Image)
	}

	HD.Active = true
	fmt.Printf("\033[1;32m🌟 GOSMS HD ENGINE:\033[0m Pack cargado -> %d texturas (Escala %dx)\n", len(HD.TileMap), HD.Scale)
}

func (hd *HDRenderer) GetHDTile(tileData []byte, cram []byte, isSprite bool) *ebiten.Image {
	if !hd.Active {
		return nil
	}
	hStr := TileHash(tileData, cram, isSprite)
	return hd.TileMap[hStr]
}
