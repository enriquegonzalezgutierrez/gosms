// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package machine

import (
	"os"
	"testing"
)

func TestSaveAndLoadState(t *testing.T) {
	rom := make([]byte, 0x8000)
	m := New(rom)

	// Modificar estados en CPU, VDP y PSG
	m.CPU.PC = 0x4567
	m.CPU.A = 0x42
	m.CPU.RAM[0x0100] = 0xFE
	m.VDP.VRAM[0x0200] = 0xBA
	m.VDP.Reg[7] = 0x05

	tmpState := "test_save.state"
	defer os.Remove(tmpState)

	// Guardar estado
	if err := m.SaveState(tmpState); err != nil {
		t.Fatalf("error al guardar estado: %v", err)
	}

	// Crear máquina nueva vacía y cargar estado
	m2 := New(rom)
	if err := m2.LoadState(tmpState); err != nil {
		t.Fatalf("error al cargar estado: %v", err)
	}

	// Verificar integridad bit a bit
	if m2.CPU.PC != 0x4567 || m2.CPU.A != 0x42 {
		t.Errorf("CPU no coincide tras LoadState: PC=%04X A=%02X", m2.CPU.PC, m2.CPU.A)
	}
	if m2.CPU.RAM[0x0100] != 0xFE {
		t.Errorf("RAM de CPU no coincide tras LoadState")
	}
	if m2.VDP.VRAM[0x0200] != 0xBA || m2.VDP.Reg[7] != 0x05 {
		t.Errorf("VDP no coincide tras LoadState")
	}
}

func TestRunFrameCycles(t *testing.T) {
	rom := make([]byte, 0x8000)
	m := New(rom)

	initialCycles := m.CPU.Cycles
	m.RunFrame()
	cyclesExecuted := m.CPU.Cycles - initialCycles

	// 262 líneas * 228 ciclos = 59.736 ciclos por frame
	expectedMin := uint64(59700)
	if cyclesExecuted < expectedMin {
		t.Errorf("ciclos por frame insuficientes: esperados >%d, obtenidos: %d", expectedMin, cyclesExecuted)
	}
}
