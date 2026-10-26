package main

import (
	"image/color"
	"testing"
)

func TestDecodeColor(t *testing.T) {
	got := decodeColor(0x39)
	want := color.RGBA{R: 85, G: 170, B: 255, A: 255}
	if got != want {
		t.Fatalf("decodeColor(0x39) = %#v, want %#v", got, want)
	}
}

func TestIsPaletteValid(t *testing.T) {
	if isPaletteValid([]byte{0, 0, 0, 0}) {
		t.Fatal("all zero palette should be invalid")
	}
	if isPaletteValid([]byte{0, 1, 1, 0}) {
		t.Fatal("palette with fewer than three nonzero entries should be invalid")
	}
	if !isPaletteValid([]byte{0, 1, 2, 3}) {
		t.Fatal("palette with three distinct nonzero entries should be valid")
	}
}

func TestDecodeTileTransparency(t *testing.T) {
	tile := make([]byte, 32)
	tile[0] = 0x80
	palette := make([]byte, 16)
	palette[1] = 0x01

	background := decodeTile(tile, palette, false)
	sprite := decodeTile(tile, palette, true)
	if got := background.RGBAAt(0, 0).A; got != 255 {
		t.Fatalf("background alpha = %d, want 255", got)
	}
	if got := background.RGBAAt(1, 0).A; got != 255 {
		t.Fatalf("background color zero alpha = %d, want 255", got)
	}
	if got := sprite.RGBAAt(0, 0).A; got != 255 {
		t.Fatalf("sprite nonzero alpha = %d, want 255", got)
	}
	if got := sprite.RGBAAt(1, 0).A; got != 0 {
		t.Fatalf("sprite color zero alpha = %d, want 0", got)
	}
}

func TestIsBlankTile(t *testing.T) {
	if !isBlankTile(make([]byte, 32)) {
		t.Fatal("zero-filled tile should be blank")
	}
	nonBlank := make([]byte, 32)
	nonBlank[17] = 1
	if isBlankTile(nonBlank) {
		t.Fatal("tile with pixel data should not be blank")
	}
}
