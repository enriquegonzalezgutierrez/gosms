// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package vdp

import "testing"

func benchmarkVDP() *VDP {
	v := New()
	// A full 32x28 name table of non-empty tiles and a simple palette keep the
	// renderer on its normal path instead of short-circuiting blank tiles.
	for i := 0; i < 0x4000; i++ {
		v.VRAM[i] = byte(i * 7)
	}
	for i := 0; i < 16; i++ {
		v.CRAM[i] = byte(i * 17)
		v.CRAM[16+i] = byte(255 - i*11)
	}
	v.Reg[2] = 0x0E // name table at $3800
	v.Reg[5] = 0x7E // sprite attribute table at $3F00
	v.Reg[7] = 0x00
	return v
}

func BenchmarkRenderLine(b *testing.B) {
	v := benchmarkVDP()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.RenderLine(i % ScreenH)
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "lines/s")
}

func BenchmarkRenderFrame(b *testing.B) {
	v := benchmarkVDP()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for line := 0; line < ScreenH; line++ {
			v.RenderLine(line)
			v.SetLine(line)
		}
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/frame")
}
