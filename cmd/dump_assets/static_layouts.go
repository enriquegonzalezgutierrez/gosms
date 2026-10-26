package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	"gosms/pkg/machine"
)

const (
	alexKiddLayoutStart = 0x18000
	alexKiddLayoutEnd   = 0x1bdb9
	alexKiddBankBase    = 0x8000
	alexKiddBankOffset  = 0x18000
	alexKiddScreenSize  = 16 * 12
)

var alexKiddLevelNames = []string{
	"Mt Ethernal Stage 1", "Mt Ethernal Stage 2", "Lake Fathom",
	"The Island of Nurai", "Lake Fathom Part 2", "The Village of Namui",
	"Mt Kave", "The Blackwoods", "River", "Bingoo Lowland",
	"Radactian Castle", "The City of Radactian", "Swamp",
	"The Kingdom of Nibana Part 1", "The Kingdom of Nibana Part 2",
	"Janken's Castle", "Cragg Lake",
}

type decodedLayoutScreen struct {
	romOffset int
	metatiles []byte
}

func slot2ROMOffset(pointer uint16) (int, bool) {
	if pointer < alexKiddBankBase || pointer >= 0xc000 {
		return 0, false
	}
	offset := alexKiddBankOffset + int(pointer-alexKiddBankBase)
	return offset, offset >= alexKiddLayoutStart && offset < alexKiddLayoutEnd
}

// decodeLayoutScreen reads one 16x12 metatile screen from the game's RLE stream.
func decodeLayoutScreen(rom []byte, offset int) ([]byte, bool) {
	if offset < alexKiddLayoutStart || offset >= alexKiddLayoutEnd {
		return nil, false
	}
	decoded := make([]byte, 0, alexKiddScreenSize)
	for cursor := offset; cursor < len(rom) && cursor < alexKiddLayoutEnd; {
		control := rom[cursor]
		cursor++
		if control == 0 {
			return decoded, len(decoded) == alexKiddScreenSize
		}
		count := int(control & 0x7f)
		if count == 0 {
			return nil, false
		}
		if control&0x80 != 0 {
			if cursor+count > len(rom) || cursor+count > alexKiddLayoutEnd || len(decoded)+count > alexKiddScreenSize {
				return nil, false
			}
			decoded = append(decoded, rom[cursor:cursor+count]...)
			cursor += count
		} else {
			if cursor >= len(rom) || len(decoded)+count > alexKiddScreenSize {
				return nil, false
			}
			for i := 0; i < count; i++ {
				decoded = append(decoded, rom[cursor])
			}
			cursor++
		}
		if len(decoded) == alexKiddScreenSize {
			return decoded, true
		}
	}
	return nil, false
}

func collectLayoutScreens(rom []byte, pointer uint16, seenTables map[int]bool, screens map[int][]byte) {
	offset, ok := slot2ROMOffset(pointer)
	if !ok {
		return
	}
	if data, valid := decodeLayoutScreen(rom, offset); valid {
		if _, exists := screens[offset]; !exists {
			screens[offset] = data
		}
		return
	}
	if seenTables[offset] {
		return
	}
	seenTables[offset] = true

	// Layout headers contain short tables of pointers to more tables or screen
	// streams. Stop at the first non-bank pointer; this avoids interpreting RLE
	// payload bytes following a table as more entries.
	for cursor := offset; cursor+1 < alexKiddLayoutEnd && cursor-offset < 64; cursor += 2 {
		child := uint16(rom[cursor]) | uint16(rom[cursor+1])<<8
		if _, valid := slot2ROMOffset(child); !valid {
			break
		}
		collectLayoutScreens(rom, child, seenTables, screens)
	}
}

func alexKiddLayoutScenes(rom []byte, sceneDir string) ([]machine.TileSceneManifest, int, error) {
	if len(rom) != 128*1024 {
		return nil, 0, fmt.Errorf("Alex Kidd estático requiere una ROM de 128 KiB; recibido %d bytes", len(rom))
	}
	if err := os.MkdirAll(sceneDir, 0755); err != nil {
		return nil, 0, err
	}
	var scenes []machine.TileSceneManifest
	totalScreens := 0
	for level := range alexKiddLevelNames {
		descriptorPointer := uint16(rom[0x66d6+level*2]) | uint16(rom[0x66d7+level*2])<<8
		descriptorOffset := int(descriptorPointer)
		if descriptorOffset+12 > len(rom) {
			return nil, 0, fmt.Errorf("descriptor fuera de ROM para nivel %d", level+1)
		}
		descriptor := rom[descriptorOffset : descriptorOffset+12]
		first := uint16(descriptor[1]) | uint16(descriptor[2])<<8
		second := uint16(descriptor[3]) | uint16(descriptor[4])<<8
		screens := make(map[int][]byte)
		tables := make(map[int]bool)
		collectLayoutScreens(rom, first, tables, screens)
		collectLayoutScreens(rom, second, tables, screens)
		if len(screens) == 0 {
			return nil, totalScreens, fmt.Errorf("no se encontraron pantallas estáticas del nivel %d (%s)", level+1, alexKiddLevelNames[level])
		}
		offsets := make([]int, 0, len(screens))
		for offset := range screens {
			offsets = append(offsets, offset)
		}
		sort.Ints(offsets)
		for screenIndex, offset := range offsets {
			metatileMap := make([][]int, 12)
			for row := range metatileMap {
				metatileMap[row] = make([]int, 16)
				for column := range metatileMap[row] {
					metatileMap[row][column] = int(screens[offset][row*16+column])
				}
			}
			name := fmt.Sprintf("level_%02d_screen_%03d_metatiles.png", level+1, screenIndex+1)
			if err := writeMetatileIndexMap(filepath.Join(sceneDir, name), screens[offset]); err != nil {
				return nil, totalScreens, err
			}
			scenes = append(scenes, machine.TileSceneManifest{
				Image: filepath.ToSlash(filepath.Join("scenes", name)),
				Level: level + 1, LevelName: alexKiddLevelNames[level],
				Screen: screenIndex + 1, ROMOffset: offset, MetatileMap: metatileMap,
			})
			totalScreens++
		}
	}
	return scenes, totalScreens, nil
}

func writeMetatileIndexMap(path string, data []byte) error {
	img := image.NewRGBA(image.Rect(0, 0, 16*16, 12*16))
	for index, value := range data {
		// A stable high-contrast index palette makes repeated metatiles easy to
		// track before the game's per-level graphics are applied.
		c := color.RGBA{R: 80 + value*73%176, G: 80 + value*151%176, B: 80 + value*199%176, A: 255}
		if value == 0 {
			c = color.RGBA{R: 38, G: 42, B: 48, A: 255}
		}
		x0, y0 := (index%16)*16, (index/16)*16
		for y := y0; y < y0+16; y++ {
			for x := x0; x < x0+16; x++ {
				img.SetRGBA(x, y, c)
			}
		}
		for x := x0; x < x0+16; x++ {
			img.SetRGBA(x, y0, color.RGBA{R: 12, G: 14, B: 18, A: 255})
		}
		for y := y0; y < y0+16; y++ {
			img.SetRGBA(x0, y, color.RGBA{R: 12, G: 14, B: 18, A: 255})
		}
		drawHexByte(img, x0+4, y0+5, value)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func drawHexByte(img *image.RGBA, x, y int, value byte) {
	font := [16][5]uint8{
		{0b111, 0b101, 0b101, 0b101, 0b111}, {0b010, 0b110, 0b010, 0b010, 0b111},
		{0b111, 0b001, 0b111, 0b100, 0b111}, {0b111, 0b001, 0b111, 0b001, 0b111},
		{0b101, 0b101, 0b111, 0b001, 0b001}, {0b111, 0b100, 0b111, 0b001, 0b111},
		{0b111, 0b100, 0b111, 0b101, 0b111}, {0b111, 0b001, 0b010, 0b010, 0b010},
		{0b111, 0b101, 0b111, 0b101, 0b111}, {0b111, 0b101, 0b111, 0b001, 0b111},
		{0b010, 0b101, 0b111, 0b101, 0b101}, {0b110, 0b101, 0b110, 0b101, 0b110},
		{0b011, 0b100, 0b100, 0b100, 0b011}, {0b110, 0b101, 0b101, 0b101, 0b110},
		{0b111, 0b100, 0b110, 0b100, 0b111}, {0b111, 0b100, 0b110, 0b100, 0b100},
	}
	for digit := 0; digit < 2; digit++ {
		valueNibble := value >> 4
		if digit == 1 {
			valueNibble = value & 0x0f
		}
		for row, bits := range font[valueNibble] {
			for col := 0; col < 3; col++ {
				if bits&(1<<uint(2-col)) == 0 {
					continue
				}
				img.SetRGBA(x+digit*4+col+1, y+row+1, color.RGBA{R: 8, G: 10, B: 12, A: 255})
				img.SetRGBA(x+digit*4+col, y+row, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
}
