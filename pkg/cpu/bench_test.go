// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package cpu

import "testing"

// benchmarkROM is a minimal 32 KiB cartridge with an infinite JR loop at $0000.
// It keeps the CPU busy without needing a real game or display hardware.
func benchmarkROM() []byte {
	rom := make([]byte, 0x8000)
	rom[0] = 0x00 // NOP
	rom[1] = 0x18 // JR
	rom[2] = 0xFD // -3: back to $0000
	return rom
}

func BenchmarkStep(b *testing.B) {
	c := NewCPU(benchmarkROM())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.Step()
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "instr/s")
}

func BenchmarkStepPorted(b *testing.B) {
	// Same loop but exercising the memory/port path used by real frames.
	c := NewCPU(benchmarkROM())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.ReadByte(0x00)
		c.WriteByte(0xC000, byte(i))
		_, _, _ = c.Step()
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "instr/s")
}
