// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"image"
	"image/color"
	"testing"

	"gosms/pkg/machine"
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

func TestMakeBackgroundObjectsDeduplicates(t *testing.T) {
	tileMap := make([][]int, 16)
	for y := range tileMap {
		tileMap[y] = make([]int, 16)
		for x := range tileMap[y] {
			tileMap[y][x] = 0
		}
	}
	tiles := []machine.TileManifest{{Hash: "tile", Index: 0}}
	images := map[string]*image.RGBA{"tile": image.NewRGBA(image.Rect(0, 0, 8, 8))}
	objects := make(map[string]*objectAccumulator)

	makeBackgroundObjects(tileMap, 10, tiles, images, objects, 8, 0)
	if len(objects) != 1 {
		t.Fatalf("got %d objects, want 1 deduplicated block", len(objects))
	}
	for _, object := range objects {
		if object.manifest.Kind != "background_object" {
			t.Fatalf("kind = %q", object.manifest.Kind)
		}
		if object.manifest.FramesSeen != 4 {
			t.Fatalf("FramesSeen = %d, want 4", object.manifest.FramesSeen)
		}
		if len(object.manifest.Tiles) != 64 {
			t.Fatalf("tiles = %d, want 64", len(object.manifest.Tiles))
		}
	}
}

func TestMakeBackgroundObjectsSkipsEmptyBlocks(t *testing.T) {
	tileMap := make([][]int, 8)
	for y := range tileMap {
		tileMap[y] = make([]int, 8)
		for x := range tileMap[y] {
			tileMap[y][x] = -1
		}
	}
	objects := make(map[string]*objectAccumulator)
	makeBackgroundObjects(tileMap, 0, nil, nil, objects, 8, 0)
	if len(objects) != 0 {
		t.Fatalf("empty background should not create objects: %d", len(objects))
	}
}

func TestGroupSpritesPerSprite(t *testing.T) {
	sprites := []visibleSprite{
		{x: 0, y: 0, width: 8, height: 8},
		{x: 8, y: 0, width: 8, height: 8},
		{x: 40, y: 40, width: 8, height: 8},
	}
	groups := groupSprites(sprites, false)
	if len(groups) != len(sprites) {
		t.Fatalf("groupSprites(per-sprite) = %d grupos, want %d", len(groups), len(sprites))
	}
	for i, group := range groups {
		if len(group) != 1 {
			t.Fatalf("grupo %d tiene %d sprites, want 1", i, len(group))
		}
	}
}

func TestGroupSpritesClustered(t *testing.T) {
	sprites := []visibleSprite{
		{x: 0, y: 0, width: 8, height: 8},
		{x: 8, y: 0, width: 8, height: 8},
		{x: 40, y: 40, width: 8, height: 8},
	}
	groups := groupSprites(sprites, true)
	if len(groups) != 2 {
		t.Fatalf("groupSprites(cluster) = %d grupos, want 2", len(groups))
	}
}
