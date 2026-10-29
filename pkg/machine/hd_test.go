// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package machine

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"gosms/pkg/vdp"
)

func TestLoadHDPackV2(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("graphics context unavailable: %v", r)
		}
	}()

	dir := t.TempDir()
	pagePath := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagePath, 0o755); err != nil {
		t.Fatal(err)
	}
	page := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			page.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	f, err := os.Create(filepath.Join(pagePath, "page_000.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, page); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	doc := HDPackDocument{
		Version:  2,
		Scale:    4,
		TileSize: 8,
		Pages:    []HDAtlasPage{{File: "pages/page_000.png", Width: 32, Height: 32}},
		Tiles: []HDAtlasTile{{
			Hash: "abc", Page: 0, X: 0, Y: 0, W: 32, H: 32,
		}},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !loadHDPackV2(dir, data) {
		t.Fatal("loadHDPackV2 returned false")
	}
	if HD == nil || !HD.Active || HD.Scale != 4 {
		t.Fatalf("unexpected renderer: %+v", HD)
	}
	if _, ok := HD.TileRefs["abc"]; !ok {
		t.Fatal("tile reference was not mapped")
	}
	if HD.Pages[0] != nil {
		t.Fatal("page should not be decoded during load")
	}
	if !HD.LoadPage(0) || HD.Pages[0] == nil {
		t.Fatal("page was not loaded")
	}
	// Out-of-bounds tiles must be ignored instead of panicking later.
	doc.Tiles[0].X = 1000
	data, _ = json.Marshal(doc)
	if loadHDPackV2(dir, data) {
		t.Fatal("out-of-bounds pack should be rejected")
	}
}

func TestDrawLayerSmoke(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("graphics context unavailable: %v", r)
		}
	}()
	v := vdp.New()
	v.Reg[1] |= 0x40 // pantalla activa
	v.Reg[2] = 0x0E  // name table en 0x3800
	for i := range v.CRAM {
		v.CRAM[i] = byte(i)
	}
	hd := &HDRenderer{Active: true, Scale: 4, TileMap: map[string]*ebiten.Image{}}
	dst := ebiten.NewImage((vdp.ScreenW-8)*4, vdp.ScreenH*4)
	hd.DrawLayer(v, dst, nil, 4, 0, 0, true)
	if dst.Bounds().Dx() != (vdp.ScreenW-8)*4 {
		t.Fatalf("unexpected destination size: %v", dst.Bounds())
	}
}
