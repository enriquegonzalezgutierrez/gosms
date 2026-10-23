package machine

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

type HDRenderer struct {
	Active   bool
	TileMap  map[string]*ebiten.Image
	Scale    int
	BaseDir  string
	OrigDir  string
	HDDir    string
	mu       sync.RWMutex
	captured sync.Map
	taskChan chan dumpTask
}

type dumpTask struct {
	hash     string
	tileData []byte
	palette  []color.RGBA
}

var HD *HDRenderer

func InitHD(romPath string) {
	baseName := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	origDir := filepath.Join(".", "assets", baseName, "original", "tiles")
	hdDir := filepath.Join(".", "assets", baseName, "hd", "tiles")

	_ = os.MkdirAll(origDir, 0755)
	_ = os.MkdirAll(hdDir, 0755)

	HD = &HDRenderer{
		Active:   true,
		TileMap:  make(map[string]*ebiten.Image),
		Scale:    4,
		BaseDir:  baseName,
		OrigDir:  origDir,
		HDDir:    hdDir,
		taskChan: make(chan dumpTask, 1000),
	}

	if origFiles, err := os.ReadDir(origDir); err == nil {
		for _, f := range origFiles {
			if strings.HasSuffix(f.Name(), ".png") {
				h := strings.TrimSuffix(strings.TrimPrefix(f.Name(), "tile_"), ".png")
				HD.captured.Store(h, true)
			}
		}
	}

	// Cargar las texturas 4K reales generadas por la IA
	if hdFiles, err := os.ReadDir(hdDir); err == nil {
		for _, f := range hdFiles {
			if strings.HasSuffix(f.Name(), ".png") {
				h := strings.TrimSuffix(strings.TrimPrefix(f.Name(), "tile_"), ".png")
				filePath := filepath.Join(hdDir, f.Name())
				if file, err := os.Open(filePath); err == nil {
					if img, err := png.Decode(file); err == nil {
						HD.TileMap[h] = ebiten.NewImageFromImage(img)
					}
					file.Close()
				}
			}
		}
	}

	fmt.Printf("\033[1;32m🌟 GOSMS HD ENGINE:\033[0m %d texturas 4K cargadas | %s\n", len(HD.TileMap), baseName)

	go HD.saverWorker()
}

func (hd *HDRenderer) saverWorker() {
	for t := range hd.taskChan {
		orig := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			b0, b1, b2, b3 := t.tileData[y*4], t.tileData[y*4+1], t.tileData[y*4+2], t.tileData[y*4+3]
			for x := 0; x < 8; x++ {
				bit := 7 - x
				cIdx := ((b0>>bit)&1) | (((b1>>bit)&1)<<1) | (((b2>>bit)&1)<<2) | (((b3>>bit)&1)<<3)
				col := t.palette[cIdx&0x0F]
				if cIdx == 0 {
					col.A = 0
				}
				orig.Set(x, y, col)
			}
		}

		origPath := filepath.Join(hd.OrigDir, fmt.Sprintf("tile_%s.png", t.hash))
		if f, err := os.Create(origPath); err == nil {
			png.Encode(f, orig)
			f.Close()
		}
	}
}

func (hd *HDRenderer) RequestCapture(tileData []byte, pal []color.RGBA, pIdx int) {
	// Solo capturamos si la paleta tiene colores vivos válidos
	nonBlack := 0
	for _, c := range pal {
		if c.R > 20 || c.G > 20 || c.B > 20 {
			nonBlack++
		}
	}
	if nonBlack < 4 {
		return
	}

	h := fnv.New64a()
	h.Write(tileData)
	h.Write([]byte{byte(pIdx)})
	hashStr := fmt.Sprintf("%016x", h.Sum64())

	if _, exists := hd.captured.LoadOrStore(hashStr, true); exists {
		return
	}

	dataCopy := make([]byte, len(tileData))
	copy(dataCopy, tileData)

	palCopy := make([]color.RGBA, len(pal))
	copy(palCopy, pal)

	select {
	case hd.taskChan <- dumpTask{hash: hashStr, tileData: dataCopy, palette: palCopy}:
	default:
	}
}

func (hd *HDRenderer) GetHDTile(tileData []byte, pIdx int) *ebiten.Image {
	if !hd.Active {
		return nil
	}
	h := fnv.New64a()
	h.Write(tileData)
	h.Write([]byte{byte(pIdx)})
	hashStr := fmt.Sprintf("%016x", h.Sum64())

	hd.mu.RLock()
	img := hd.TileMap[hashStr]
	hd.mu.RUnlock()

	return img
}
