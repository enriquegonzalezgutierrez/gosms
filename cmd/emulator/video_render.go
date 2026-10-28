package main

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"gosms/pkg/machine"
	"gosms/pkg/vdp"
	"gosms/pkg/video"
)

// frameSource returns the image that should be fed to the post-processing
// pipeline and the rectangle to sample from it.
func (g *game) frameSource() (*ebiten.Image, image.Rectangle) {
	if g.hdMode && machine.HD != nil && machine.HD.Active {
		return g.hdFrameSource()
	}
	if g.videoFilter == VideoFilterEPX {
		return g.epxFrameSource()
	}
	if g.crop {
		return g.img, image.Rect(8, 0, vdp.ScreenW, vdp.ScreenH)
	}
	return g.img, image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH)
}

// hdFrameSource composes the remastered tile layer at its native pack scale.
// The pipeline then scales it to the window, so the HD pack resolution is
// independent from the window size.
func (g *game) hdFrameSource() (*ebiten.Image, image.Rectangle) {
	hd := machine.HD
	minX := 0
	if g.crop {
		minX = 8
	}
	w := (vdp.ScreenW - minX) * hd.Scale
	h := vdp.ScreenH * hd.Scale
	if w < 1 || h < 1 {
		return g.img, image.Rect(0, 0, vdp.ScreenW, vdp.ScreenH)
	}
	if g.hdTarget == nil || g.hdTarget.Bounds().Dx() != w || g.hdTarget.Bounds().Dy() != h {
		g.hdTarget = ebiten.NewImage(w, h)
	} else {
		g.hdTarget.Clear()
	}
	// drawHDLayer uses the same coordinate system as the emulated 256x192
	// screen, scaled by the pack factor. Passing 0,0 as the destination keeps
	// the composed layer aligned with its own texture.
	g.drawHDLayer(g.hdTarget, float64(hd.Scale), 0, 0)
	return g.hdTarget, image.Rect(0, 0, w, h)
}

// epxFrameSource keeps the CPU Scale2x path available as a lightweight
// alternative for low-power devices.
func (g *game) epxFrameSource() (*ebiten.Image, image.Rectangle) {
	if g.epxImg == nil {
		g.epxImg = ebiten.NewImage(vdp.ScreenW*2, vdp.ScreenH*2)
		g.epxBuf = make([]byte, vdp.ScreenW*2*vdp.ScreenH*2*4)
	}
	src := g.m.VDP.Frame[:]
	dst := g.epxBuf
	srcW := vdp.ScreenW
	srcH := vdp.ScreenH
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			idxP := (y*srcW + x) * 4
			idxA, idxB, idxI, idxD := idxP, idxP, idxP, idxP
			if y > 0 {
				idxA = ((y-1)*srcW + x) * 4
			}
			if y < srcH-1 {
				idxB = ((y+1)*srcW + x) * 4
			}
			if x > 0 {
				idxI = (y*srcW + x - 1) * 4
			}
			if x < srcW-1 {
				idxD = (y*srcW + x + 1) * 4
			}

			p := uint32(src[idxP])<<16 | uint32(src[idxP+1])<<8 | uint32(src[idxP+2])
			a := uint32(src[idxA])<<16 | uint32(src[idxA+1])<<8 | uint32(src[idxA+2])
			b := uint32(src[idxB])<<16 | uint32(src[idxB+1])<<8 | uint32(src[idxB+2])
			i := uint32(src[idxI])<<16 | uint32(src[idxI+1])<<8 | uint32(src[idxI+2])
			d := uint32(src[idxD])<<16 | uint32(src[idxD+1])<<8 | uint32(src[idxD+2])

			p1, p2, p3, p4 := p, p, p, p
			if i == a && i != b && a != d {
				p1 = a
			}
			if a == d && a != i && d != b {
				p2 = a
			}
			if i == b && i != a && b != d {
				p3 = b
			}
			if b == d && b != i && d != a {
				p4 = b
			}

			dIdx1 := ((y*2)*(srcW*2) + (x * 2)) * 4
			dIdx2 := dIdx1 + 4
			dIdx3 := ((y*2+1)*(srcW*2) + (x * 2)) * 4
			dIdx4 := dIdx3 + 4

			dst[dIdx1], dst[dIdx1+1], dst[dIdx1+2], dst[dIdx1+3] = byte(p1>>16), byte(p1>>8), byte(p1), 255
			dst[dIdx2], dst[dIdx2+1], dst[dIdx2+2], dst[dIdx2+3] = byte(p2>>16), byte(p2>>8), byte(p2), 255
			dst[dIdx3], dst[dIdx3+1], dst[dIdx3+2], dst[dIdx3+3] = byte(p3>>16), byte(p3>>8), byte(p3), 255
			dst[dIdx4], dst[dIdx4+1], dst[dIdx4+2], dst[dIdx4+3] = byte(p4>>16), byte(p4>>8), byte(p4), 255
		}
	}
	g.epxImg.WritePixels(g.epxBuf)
	if g.crop {
		return g.epxImg, image.Rect(16, 0, vdp.ScreenW*2, vdp.ScreenH*2)
	}
	return g.epxImg, image.Rect(0, 0, vdp.ScreenW*2, vdp.ScreenH*2)
}

func (g *game) pipelineOptions() video.Options {
	var preset video.Preset
	switch g.videoFilter {
	case VideoFilterScanlines:
		preset = video.PresetCRT
	case VideoFilterEPX, VideoFilterPixelPerfect:
		preset = video.PresetPixelPerfect
	case VideoFilterSmooth:
		preset = video.PresetSmooth
	case VideoFilterMonochrome:
		preset = video.PresetMonochrome
	case VideoFilterRemaster2026:
		preset = video.PresetModern
	default:
		preset = video.PresetPixelPerfect
	}
	opts := video.DefaultOptions(preset, g.hdMode)
	if g.videoFilter == VideoFilterScanlines {
		// Keep the old scanline filter semantics: only the grid, no bloom or
		// colour grading. This makes the two modes easy to compare.
		opts.Bloom = 0
		opts.Sharpness = 0
		opts.Saturation = 1
		opts.Contrast = 1
		opts.Gamma = 1
		opts.Brightness = 1
		opts.Mask = 0
		opts.Vignette = 0
		opts.Curvature = 0
		opts.Scanline = 0.22
	}
	return opts
}
