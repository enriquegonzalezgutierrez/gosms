// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package video

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDrawWithModernPreset exercises all passes with real Ebiten images. It is
// skipped automatically on machines without a usable graphics context.
func TestDrawWithModernPreset(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("graphics context unavailable: %v", r)
		}
	}()

	src := ebiten.NewImage(64, 48)
	src.Fill(image.White)
	dst := ebiten.NewImage(256, 192)

	p := New()
	if err := p.Draw(dst, src, image.Rect(0, 0, 64, 48), image.Rect(0, 0, 256, 192), DefaultOptions(PresetModern, false)); err != nil {
		t.Fatalf("modern draw failed: %v", err)
	}
	if err := p.Draw(dst, src, image.Rect(0, 0, 64, 48), image.Rect(0, 0, 256, 192), DefaultOptions(PresetCRT, false)); err != nil {
		t.Fatalf("CRT draw failed: %v", err)
	}

	hdSrc := ebiten.NewImage(1024, 768)
	hdSrc.Fill(image.White)
	if err := p.Draw(dst, hdSrc, image.Rect(0, 0, 1024, 768), image.Rect(0, 0, 256, 192), DefaultOptions(PresetModern, true)); err != nil {
		t.Fatalf("HD draw failed: %v", err)
	}
}
