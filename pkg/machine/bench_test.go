// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package machine

import "testing"

func benchmarkROM() []byte {
	rom := make([]byte, 0x8000)
	rom[0] = 0x00 // NOP
	rom[1] = 0x18 // JR
	rom[2] = 0xFD // -3: back to $0000
	return rom
}

func BenchmarkRunFrame(b *testing.B) {
	m := New(benchmarkROM())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RunFrame()
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/frame")
}

func BenchmarkRunCycles(b *testing.B) {
	m := New(benchmarkROM())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RunCycles(1000)
	}
	b.ReportMetric(float64(b.N*1000)/b.Elapsed().Seconds(), "cycles/s")
}
