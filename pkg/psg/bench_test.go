// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package psg

import "testing"

func BenchmarkRender(b *testing.B) {
	p := New(44100)
	p.Write(0x80 | 0x0F)       // canal 0, volumen máximo
	p.Write(0x00)              // frecuencia baja
	p.Write(0x01)              // frecuencia alta
	buf := make([]byte, 801*2) // un frame NTSC a 48 kHz
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Render(buf)
	}
	b.ReportMetric(float64(b.N*801)/b.Elapsed().Seconds(), "samples/s")
}
