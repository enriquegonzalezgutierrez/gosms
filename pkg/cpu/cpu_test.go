// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package cpu

import (
	"os"
	"testing"
)

func TestNMI(t *testing.T) {
	rom := make([]byte, 0x8000)
	c := NewCPU(rom)
	c.PC = 0x1234
	c.IFF1 = true
	c.NMI = true

	name, cyc, ok := c.Step()
	if !ok || name != "<NMI>" {
		t.Fatalf("se esperaba <NMI>, obtenido: %s", name)
	}
	if c.PC != 0x0066 {
		t.Errorf("PC tras NMI debería ser 0x0066, obtenido: 0x%04X", c.PC)
	}
	if c.IFF1 != false || c.IFF2 != true {
		t.Errorf("flags IFF1=%v, IFF2=%v incorrectos tras NMI", c.IFF1, c.IFF2)
	}
	if cyc != 11 {
		t.Errorf("ciclos NMI deberían ser 11, obtenidos: %d", cyc)
	}
}

func TestROMBanking(t *testing.T) {
	// ROM de 64 KB (4 bancos de 16 KB)
	rom := make([]byte, 0x10000)
	rom[0*BankSize] = 0x10 // Banco 0
	rom[1*BankSize] = 0x20 // Banco 1
	rom[2*BankSize] = 0x30 // Banco 2
	rom[3*BankSize] = 0x40 // Banco 3

	c := NewCPU(rom)

	// Por defecto Bank 1 está en $4000..$7FFF (Slot 1)
	if val := c.ReadByte(0x4000); val != 0x20 {
		t.Errorf("Slot 1 inicial debería ser 0x20, obtenido: 0x%02X", val)
	}

	// Mapear Banco 3 a Slot 1 escribiendo en $FFFE
	c.WriteByte(0xFFFE, 3)
	if val := c.ReadByte(0x4000); val != 0x40 {
		t.Errorf("Slot 1 tras remapeo debería ser 0x40, obtenido: 0x%02X", val)
	}
}

func TestCartridgeSRAM(t *testing.T) {
	rom := make([]byte, 0x8000)
	c := NewCPU(rom)
	tmpSave := "test_save.sav"
	defer os.Remove(tmpSave)

	c.SRAMPath = tmpSave

	// Sin activar bit 3 de $FFFC, $8000 es ROM de solo lectura
	c.WriteByte(0x8000, 0xAA)
	if val := c.ReadByte(0x8000); val == 0xAA {
		t.Fatal("no se debería poder escribir en ROM")
	}

	// Activar Cartridge RAM (bit 3 de $FFFC)
	c.WriteByte(0xFFFC, 0x08)
	c.WriteByte(0x8000, 0x55)
	if val := c.ReadByte(0x8000); val != 0x55 {
		t.Fatalf("SRAM debería contener 0x55, obtenido: 0x%02X", val)
	}

	// Probar persistencia SaveSRAM y LoadSRAM
	c.SaveSRAM()
	c2 := NewCPU(rom)
	c2.LoadSRAM(tmpSave)
	c2.WriteByte(0xFFFC, 0x08)
	if val := c2.ReadByte(0x8000); val != 0x55 {
		t.Errorf("SRAM cargada de disco debería ser 0x55, obtenido: 0x%02X", val)
	}
}

func TestIOPortMirroring(t *testing.T) {
	rom := make([]byte, 0x8000)
	c := NewCPU(rom)

	// Puertos $40..$7F deben escribir al PSG sin errores
	c.WritePort(0x40, 0x90) // Latch volumen canal 0
	c.WritePort(0x7F, 0x9F) // Silencio canal 0

	// Puertos $80..$BF deben ser VDP
	c.WritePort(0xBF, 0x05) // Primer byte control VDP
	c.WritePort(0xBF, 0x80) // Segundo byte: escribir Reg 0 = 0x05
	if c.VDP.Reg[0] != 0x05 {
		t.Errorf("Reg[0] del VDP debería ser 0x05, obtenido: 0x%02X", c.VDP.Reg[0])
	}
}
