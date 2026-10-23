package vdp

import "testing"

func TestVDPRegisters(t *testing.T) {
	v := New()

	// Escribir en Registro 7 (color de fondo = 0x0C)
	v.WriteControl(0x0C)
	v.WriteControl(0x87)

	if v.Reg[7] != 0x0C {
		t.Errorf("Reg[7] debería ser 0x0C, obtenido: 0x%02X", v.Reg[7])
	}
}

func TestVRAMReadWrite(t *testing.T) {
	v := New()

	// Configurar dirección VRAM 0x1234 para escritura (código 1)
	v.WriteControl(0x34)
	v.WriteControl(0x40 | 0x12)

	// Escribir 0xAB en VRAM
	v.WriteData(0xAB)

	if v.VRAM[0x1234] != 0xAB {
		t.Errorf("VRAM[0x1234] debería ser 0xAB, obtenido: 0x%02X", v.VRAM[0x1234])
	}
}

func TestCRAMPalette(t *testing.T) {
	v := New()

	// Configurar dirección CRAM índice 0 (código 3)
	v.WriteControl(0x00)
	v.WriteControl(0xC0)

	// Escribir color con R=3, G=0, B=0 (valor 0x03 en CRAM)
	v.WriteData(0x03)

	rgb := v.color(0)
	// R = 3 * 85 = 255 (0xFF0000)
	expected := uint32(0xFF0000)
	if rgb != expected {
		t.Errorf("color CRAM RGB esperado 0x%06X, obtenido: 0x%06X", expected, rgb)
	}
}
