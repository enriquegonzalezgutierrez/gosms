// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package machine

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"gosms/pkg/vdp"
)

// TestLoadHDPackHeights checks that a v2 pack with parallel height pages is
// accepted and exposes them lazily, and that a pack without them stays valid
// but reports no height channel.
func TestLoadHDPackHeights(t *testing.T) {
	tile := `{"hash":"0000000000000001","page":0,"x":2,"y":2,"w":32,"h":32,"is_sprite":false}`

	cases := []struct {
		name       string
		heightJSON string
		wantHeight bool
	}{
		{"with heights", `,"height_pages":[{"file":"pages/height_000.png","width":512,"height":512}]`, true},
		{"colour only", ``, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			doc := `{"version":2,"scale":4,"pages":[{"file":"pages/page_000.png","width":512,"height":512}]` +
				tc.heightJSON + `,"tiles":[` + tile + `]}`
			if err := os.WriteFile(filepath.Join(dir, "pack.json"), []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}

			previous := HD
			defer func() { HD = previous }()
			if !loadHDPackV2(dir, []byte(doc)) {
				t.Fatalf("pack rejected")
			}
			if HD.HasHeight != tc.wantHeight {
				t.Fatalf("HasHeight = %v, want %v", HD.HasHeight, tc.wantHeight)
			}
			if tc.wantHeight {
				if len(HD.HeightPagePaths) != 1 || HD.HeightPagePaths[0] != filepath.Join(dir, "pages/height_000.png") {
					t.Fatalf("unexpected height paths: %v", HD.HeightPagePaths)
				}
				if HD.LoadHeightPage(5) {
					t.Fatalf("out-of-range height page should not load")
				}
				// The page file does not exist: loading must fail cleanly, not panic.
				if HD.LoadHeightPage(0) {
					t.Fatalf("missing height page should not load")
				}
			} else if HD.GetHDHeight([]byte{1}, []byte{0}, false) != nil {
				t.Fatalf("GetHDHeight should return nil without a height channel")
			}
		})
	}
}

// TestGradeColor checks the pack palette translation used by the VDP backdrop
// and by the dynamic fallback tiles.
func TestGradeColor(t *testing.T) {
	hd := &HDRenderer{}
	blue := color.RGBA{0, 0, 255, 255}
	if got := hd.GradeColor(blue); got != blue {
		t.Fatalf("without palette GradeColor changed %v to %v", blue, got)
	}

	steel := color.RGBA{52, 99, 156, 255}
	hd.Palette = map[color.RGBA]color.RGBA{blue: steel}
	if got := hd.GradeColor(blue); got != steel {
		t.Fatalf("GradeColor(blue) = %v, want %v", got, steel)
	}
	unknown := color.RGBA{85, 0, 0, 255}
	if got := hd.GradeColor(unknown); got != unknown {
		t.Fatalf("GradeColor should pass unknown colours through, got %v", got)
	}
}

// TestBackdrop checks that the VDP backdrop is translated with the pack
// palette. The sky pass keys on this colour, so it must match the atlas.
func TestBackdrop(t *testing.T) {
	v := &vdp.VDP{}
	v.CRAM[16] = 0x30 // azul puro: bits 4-5 = 3
	v.Reg[7] = 0

	hd := &HDRenderer{}
	if got := hd.Backdrop(v); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("Backdrop without palette = %v, want pure blue", got)
	}

	steel := color.RGBA{52, 99, 156, 255}
	hd.Palette = map[color.RGBA]color.RGBA{{0, 0, 255, 255}: steel}
	if got := hd.Backdrop(v); got != steel {
		t.Fatalf("Backdrop = %v, want %v", got, steel)
	}
}

// TestIsSkyColor keeps the sky pass out of dark caves and red backdrops.
func TestIsSkyColor(t *testing.T) {
	cases := []struct {
		name string
		c    color.RGBA
		want bool
	}{
		{"steel sky", color.RGBA{52, 99, 156, 255}, true},
		{"pure blue", color.RGBA{0, 0, 255, 255}, true},
		{"black cave", color.RGBA{26, 21, 18, 255}, false},
		{"red cave", color.RGBA{126, 28, 22, 255}, false},
		{"bone", color.RGBA{230, 223, 206, 255}, false},
	}
	for _, tc := range cases {
		if got := IsSkyColor(tc.c); got != tc.want {
			t.Errorf("IsSkyColor(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
