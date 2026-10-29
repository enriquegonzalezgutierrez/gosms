// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package video

import "testing"

func TestShadersCompile(t *testing.T) {
	p := New()
	if err := p.compileAll(); err != nil {
		t.Fatalf("shader compilation failed: %v", err)
	}
}

func TestDefaultOptions(t *testing.T) {
	modern := DefaultOptions(PresetModern, false)
	if modern.Bloom <= 0 || modern.Sharpness <= 0 {
		t.Fatalf("modern preset should enable bloom and sharpening: %+v", modern)
	}
	if modern.Lines != 192 {
		t.Fatalf("default line count = %v, want 192", modern.Lines)
	}

	pixel := DefaultOptions(PresetPixelPerfect, true)
	if pixel.Bloom != 0 || pixel.Scanline != 0 {
		t.Fatalf("pixel-perfect preset should not enable effects: %+v", pixel)
	}
}

func TestModernPlusPresets(t *testing.T) {
	modern := DefaultOptions(PresetModern, true)
	plus := DefaultOptions(PresetModernPlus, true)
	if plus.Bloom <= modern.Bloom || plus.Saturation <= modern.Saturation || plus.Contrast <= modern.Contrast {
		t.Fatalf("modern+ should push bloom/saturation/contrast: modern=%+v plus=%+v", modern, plus)
	}
}
