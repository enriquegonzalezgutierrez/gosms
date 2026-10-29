// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package video

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func benchmarkDraw(b *testing.B, preset Preset, hd bool, srcW, srcH, dstW, dstH int) {
	src := ebiten.NewImage(srcW, srcH)
	src.Fill(image.White)
	dst := ebiten.NewImage(dstW, dstH)
	p := New()
	opts := DefaultOptions(preset, hd)
	srcRect := image.Rect(0, 0, srcW, srcH)
	dstRect := image.Rect(0, 0, dstW, dstH)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.Draw(dst, src, srcRect, dstRect, opts); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/frame")
}

func BenchmarkDraw(b *testing.B) {
	for _, preset := range []struct {
		name string
		val  Preset
	}{
		{"pixel", PresetPixelPerfect},
		{"smooth", PresetSmooth},
		{"modern", PresetModern},
		{"crt", PresetCRT},
		{"mono", PresetMonochrome},
	} {
		b.Run(preset.name, func(b *testing.B) {
			benchmarkDraw(b, preset.val, false, 256, 192, 1920, 1440)
		})
	}
}

func BenchmarkDrawHD(b *testing.B) {
	for _, preset := range []struct {
		name string
		val  Preset
	}{
		{"pixel", PresetPixelPerfect},
		{"modern", PresetModern},
		{"crt", PresetCRT},
	} {
		b.Run(preset.name, func(b *testing.B) {
			benchmarkDraw(b, preset.val, true, 1024, 768, 1920, 1440)
		})
	}
}
