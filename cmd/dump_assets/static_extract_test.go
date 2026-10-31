// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"testing"
)

func TestDecodePhantasyStarTilesInterleavesFourPlanes(t *testing.T) {
	var stream []byte
	for plane := byte(0); plane < 4; plane++ {
		stream = append(stream, 0x88)
		for row := byte(0); row < 8; row++ {
			stream = append(stream, plane*8+row+1)
		}
		stream = append(stream, 0)
	}

	tiles, err := decodePhantasyStarTiles(stream, 0, len(stream))
	if err != nil {
		t.Fatalf("decodePhantasyStarTiles() error = %v", err)
	}
	if len(tiles) != 1 {
		t.Fatalf("decoded tile count = %d, want 1", len(tiles))
	}
	for row := 0; row < 8; row++ {
		for plane := 0; plane < 4; plane++ {
			want := byte(plane*8 + row + 1)
			if got := tiles[0][row*4+plane]; got != want {
				t.Errorf("tile row %d plane %d = %d, want %d", row, plane, got, want)
			}
		}
	}
}

func TestDecodeRaw3BPPAddsFourthPlane(t *testing.T) {
	data := make([]byte, 24)
	for i := range data {
		data[i] = byte(i + 1)
	}
	tiles, err := decodeRaw3BPP(data, 0, len(data))
	if err != nil {
		t.Fatalf("decodeRaw3BPP() error = %v", err)
	}
	if len(tiles) != 1 {
		t.Fatalf("decoded tile count = %d, want 1", len(tiles))
	}
	for row := 0; row < 8; row++ {
		for plane := 0; plane < 3; plane++ {
			if got, want := tiles[0][row*4+plane], data[row*3+plane]; got != want {
				t.Errorf("tile row %d plane %d = %d, want %d", row, plane, got, want)
			}
		}
		if got := tiles[0][row*4+3]; got != 0 {
			t.Errorf("tile row %d fourth plane = %d, want 0", row, got)
		}
	}
}

func TestDecodeLayoutScreenAndFollowPointer(t *testing.T) {
	rom := make([]byte, 128*1024)
	rom[alexKiddLayoutStart] = 0x02
	rom[alexKiddLayoutStart+1] = 0x80 // Pointer to the screen stream at 0x18002.
	stream := []byte{0x7f, 0x2a, 0x41, 0x2a, 0x00}
	copy(rom[alexKiddLayoutStart+2:], stream)

	decoded, ok := decodeLayoutScreen(rom, alexKiddLayoutStart+2)
	if !ok {
		t.Fatal("decodeLayoutScreen() did not accept a 192-cell screen")
	}
	want := bytes.Repeat([]byte{0x2a}, alexKiddScreenSize)
	if !bytes.Equal(decoded, want) {
		t.Fatal("decoded screen cells differ from the RLE source")
	}

	screens := make(map[int][]byte)
	collectLayoutScreens(rom, 0x8000, make(map[int]bool), screens)
	if len(screens) != 1 {
		t.Fatalf("collected screen count = %d, want 1", len(screens))
	}
	if _, ok := screens[alexKiddLayoutStart+2]; !ok {
		t.Fatal("pointer table did not resolve the screen stream")
	}
}

func TestSlot2ROMOffsetUsesAlexKiddBankWindow(t *testing.T) {
	for pointer, want := range map[uint16]int{0x8000: 0x18000, 0xbdb8: 0x1bdb8} {
		got, ok := slot2ROMOffset(pointer)
		if !ok || got != want {
			t.Errorf("slot2ROMOffset(%#04x) = (%#x, %t), want (%#x, true)", pointer, got, ok, want)
		}
	}
	if _, ok := slot2ROMOffset(0x7fff); ok {
		t.Fatal("slot2ROMOffset accepted an address outside slot 2")
	}
}

func TestAlexKiddStaticPoseParsesTileSetAndSignedOffsets(t *testing.T) {
	rom := make([]byte, 128*1024)
	// Tabla de tile sets en $10000: la entrada 0 apunta al descriptor $8010.
	rom[0x10000] = 0x10
	rom[0x10001] = 0x80
	// Descriptor de tiles en $10010: un tile, puntero $8020.
	rom[0x10010] = 1
	rom[0x10011] = 0x20
	rom[0x10012] = 0x80
	for i := 0; i < 24; i++ {
		rom[0x10020+i] = byte(i + 1)
	}

	tiles, err := alexKiddStaticTileSet(rom, 0)
	if err != nil {
		t.Fatalf("alexKiddStaticTileSet() error = %v", err)
	}
	if len(tiles) != 1 {
		t.Fatalf("tile count = %d, want 1", len(tiles))
	}
	if got := tiles[0][0]; got != 1 {
		t.Errorf("first byte = %d, want 1", got)
	}

	// Pose en $9000: dos sprites, el segundo con X negativa (-8) para probar
	// que el objeto se normaliza a un lienzo de 16x16.
	pose := []byte{0x00, 0x02, 0x00, 0x00, 0x08, 0x00, 0x00, 0xF8, 0x00}
	copy(rom[0x9000:], pose)
	def, err := alexKiddSpritePose(rom, "prueba", 0x9000)
	if err != nil {
		t.Fatalf("alexKiddSpritePose() error = %v", err)
	}
	if def.Width != 16 || def.Height != 16 {
		t.Fatalf("pose size = %dx%d, want 16x16", def.Width, def.Height)
	}
	if def.Parts[1].X != 0 || def.Parts[0].X != 8 {
		t.Errorf("normalized X = %d,%d, want 8,0", def.Parts[0].X, def.Parts[1].X)
	}
}
