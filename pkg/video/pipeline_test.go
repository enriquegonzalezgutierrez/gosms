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
