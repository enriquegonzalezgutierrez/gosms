package cpu

import (
	"fmt"

	"gosms/pkg/psg"
	"gosms/pkg/vdp"
)

const (
	BankSize = 0x4000
)

// CPU implementa el Z80 de la Sega Master System.
type CPU struct {
	PC, SP, IX, IY                 uint16
	A, F, B, C, D, E, H, L         byte
	A2, F2, B2, C2, D2, E2, H2, L2 byte
	I, R                           byte
	IFF1, IFF2                     bool
	IM                             byte
	Halted                         bool
	eiPending                      bool

	IRQ    bool // interrupción pendiente (nivel)
	Cycles uint64
	Instr  uint64

	ROM  []byte
	RAM  [0x2000]byte
	Bank [3]byte // registros del mapper: $FFFD, $FFFE, $FFFF

	VDP *vdp.VDP
	PSG *psg.PSG

	Pad1, Pad2 byte
	Periph     byte // puerto $3E/$3F (RAM/IO control)

	// Estado interno de decodificación (prefijos DD/FD) y traza
	ix      *uint16
	ixName  string
	memName string
	opLen   int
	opBytes [4]byte
	extra   int // ciclos extra de instrucciones de bloque
}

// NewCPU crea una CPU con una ROM y un VDP/PSG asociados.
func NewCPU(rom []byte) *CPU {
	return &CPU{
		SP:      0xDFF0,
		ROM:     rom,
		Bank:    [3]byte{0, 1, 2},
		ixName:  "HL",
		memName: "(HL)",
		VDP:     vdp.New(),
		PSG:     psg.New(44100),
		Pad1:    0xFF,
		Pad2:    0xFF,
	}
}

// ---------- Memoria ----------

func (c *CPU) rom(p int) byte {
	if len(c.ROM) == 0 {
		return 0xFF
	}
	if p < 0 {
		p = -p
	}
	return c.ROM[p%len(c.ROM)]
}

func (c *CPU) ReadByte(addr uint16) byte {
	switch {
	case addr < 0x0400: // los primeros 1 KB nunca se paginan
		return c.rom(int(addr))
	case addr < 0x4000:
		return c.rom(int(c.Bank[0])*BankSize + int(addr))
	case addr < 0x8000:
		return c.rom(int(c.Bank[1])*BankSize + int(addr-0x4000))
	case addr < 0xC000:
		return c.rom(int(c.Bank[2])*BankSize + int(addr-0x8000))
	default:
		return c.RAM[(addr-0xC000)&0x1FFF]
	}
}

func (c *CPU) WriteByte(addr uint16, val byte) {
	if WatchAddr != 0 && addr == WatchAddr && MemWatch != nil {
		MemWatch(addr, val, c)
	}
	if addr < 0xC000 {
		return // ROM (no hay RAM de cartucho)
	}
	if addr >= 0xFFFD { // registros del mapper Sega
		n := len(c.ROM) / BankSize
		if n > 0 {
			c.Bank[addr-0xFFFD] = byte(int(val) % n)
		}
		// Los registros del mapper son write-only, pero la RAM subyacente
		// también recibe el valor: algunos juegos releen $FFFF para
		// salvar/restaurar el banco (p. ej. GoSMS en su rutina de IRQ).
	}
	c.RAM[(addr-0xC000)&0x1FFF] = val
}

func (c *CPU) fetch8() byte {
	v := c.ReadByte(c.PC)
	c.PC++
	if c.opLen < len(c.opBytes) {
		c.opBytes[c.opLen] = v
	}
	c.opLen++
	return v
}

func (c *CPU) fetch16() uint16 {
	lo := uint16(c.fetch8())
	hi := uint16(c.fetch8())
	return hi<<8 | lo
}

// Push16 apila un valor.
func (c *CPU) Push16(v uint16) {
	c.SP--
	c.WriteByte(c.SP, byte(v>>8))
	c.SP--
	c.WriteByte(c.SP, byte(v))
}

// Pop16 desapila un valor.
func (c *CPU) Pop16() uint16 {
	lo := uint16(c.ReadByte(c.SP))
	c.SP++
	hi := uint16(c.ReadByte(c.SP))
	c.SP++
	return hi<<8 | lo
}

// ---------- Pares de registros ----------

func (c *CPU) bc() uint16 { return uint16(c.B)<<8 | uint16(c.C) }
func (c *CPU) de() uint16 { return uint16(c.D)<<8 | uint16(c.E) }
func (c *CPU) hl() uint16 { return uint16(c.H)<<8 | uint16(c.L) }

func (c *CPU) setBC(v uint16) { c.B, c.C = byte(v>>8), byte(v) }
func (c *CPU) setDE(v uint16) { c.D, c.E = byte(v>>8), byte(v) }
func (c *CPU) setHL(v uint16) { c.H, c.L = byte(v>>8), byte(v) }

// ---------- Puertos de E/S ----------

func (c *CPU) ReadPort(port byte) byte {
	switch port {
	case 0xBE:
		return c.VDP.ReadData()
	case 0xBF:
		return c.VDP.ReadStatus()
	case 0x7E:
		return c.VDP.VCounter()
	case 0x7F:
		return c.VDP.HCounter()
	case 0xDC, 0xC0:
		return c.Pad1
	case 0xDD, 0xC1:
		return c.Pad2
	case 0x3E, 0x3F:
		return c.Periph
	}
	return 0xFF
}

func (c *CPU) WritePort(port, val byte) {
	switch port {
	case 0xBE:
		c.VDP.WriteData(val)
	case 0xBF:
		c.VDP.WriteControl(val)
	case 0x7E, 0x7F:
		c.PSG.Write(val)
	case 0x3E:
		c.Periph = val
	case 0x3F:
		c.Periph = val
	}
}

// ---------- Paso de ejecución ----------

// WatchAddr y MemWatch permiten observar escrituras a una dirección de RAM.
var WatchAddr uint16
var MemWatch func(addr uint16, val byte, c *CPU)

// Step ejecuta una instrucción y devuelve su nombre, ciclos y si continuó.
func (c *CPU) Step() (string, int, bool) {
	c.opLen = 0
	c.extra = 0
	c.ix, c.ixName, c.memName = nil, "HL", "(HL)"
	c.Instr++

	// Aceptación de interrupciones (con retardo de una instrucción tras EI).
	if c.eiPending {
		c.eiPending = false
	} else if c.IRQ && c.IFF1 {
		c.IFF1, c.IFF2 = false, false
		c.IRQ = false
		c.Push16(c.PC)
		switch c.IM {
		case 2:
			c.PC = uint16(c.I)<<8 | 0xFF
		default:
			c.PC = 0x0038
		}
		c.Cycles += 13
		return "<IRQ>", 13, true
	}

	if c.Halted {
		c.Cycles += 4
		return "HALT", 4, true
	}

	pcBefore := c.PC
	op := c.fetch8()
	c.R = c.R&0x80 | (c.R+1)&0x7F
	name, ok := c.execMain(op)
	cyc := c.tstates(op, pcBefore, c.PC)
	c.Cycles += uint64(cyc)
	return name, cyc, ok
}

// StepTrace ejecuta una instrucción y devuelve una línea de traza.
func (c *CPU) StepTrace() (string, bool) {
	pc := c.PC
	name, cycles, ok := c.Step()
	raw := ""
	for i := 0; i < c.opLen && i < len(c.opBytes); i++ {
		raw += fmt.Sprintf("%02X ", c.opBytes[i])
	}
	if !ok {
		return fmt.Sprintf("[%04X] %-12s %-24s *** DETENIDO ***", pc, raw, name), false
	}
	return fmt.Sprintf("[%04X] %-12s %-26s AF=%02X%02X BC=%04X DE=%04X HL=%04X SP=%04X IX=%04X [%s] cyc=%d",
		pc, raw, name, c.A, c.F, c.bc(), c.de(), c.hl(), c.SP, c.IX, flagStr(c.F), cycles), true
}

// ---------- Contabilidad de ciclos ----------

func taken(pcBefore, pcAfter uint16, ln int) bool {
	return pcAfter != pcBefore+uint16(ln)
}

func (c *CPU) tstates(op byte, pcBefore, pcAfter uint16) int {
	var base int
	switch op {
	case 0xCB:
		sub := c.opBytes[1]
		if sub&7 == 6 {
			if sub>>6 == 1 {
				base = 12 // BIT (HL)
			} else {
				base = 15
			}
		} else {
			base = 8
		}
	case 0xED:
		base = edCycles(c.opBytes[1])
	case 0xDD, 0xFD:
		base = idxCycles(c.opBytes[:])
	default:
		base = mainCycles(op, pcBefore, pcAfter)
	}
	return base + c.extra
}

func mainCycles(op byte, pcBefore, pcAfter uint16) int {
	x, y, z := op>>6, (op>>3)&7, op&7
	p, q := y>>1, y&1
	switch x {
	case 0:
		switch z {
		case 0:
			switch y {
			case 0, 1:
				return 4
			case 2:
				if taken(pcBefore, pcAfter, 2) {
					return 13
				}
				return 8
			case 3:
				return 12
			default:
				if taken(pcBefore, pcAfter, 2) {
					return 12
				}
				return 7
			}
		case 1:
			if q == 0 {
				return 10
			}
			return 11
		case 2:
			switch p {
			case 2:
				return 16
			case 3:
				return 13
			}
			return 7
		case 3:
			return 6
		case 4, 5:
			if y == 6 {
				return 11
			}
			return 4
		case 6:
			if y == 6 {
				return 10
			}
			return 7
		default:
			return 4
		}
	case 1:
		if op == 0x76 {
			return 4
		}
		if y == 6 || z == 6 {
			return 7
		}
		return 4
	case 2:
		if z == 6 {
			return 7
		}
		return 4
	default:
		switch z {
		case 0:
			if taken(pcBefore, pcAfter, 1) {
				return 11
			}
			return 5
		case 1:
			if q == 0 {
				return 10
			}
			switch p {
			case 0:
				return 10
			case 1, 2:
				return 4
			default:
				return 6
			}
		case 2:
			return 10
		case 3:
			switch y {
			case 0:
				return 10
			case 2, 3:
				return 11
			case 4:
				return 19
			default:
				return 4
			}
		case 4:
			if taken(pcBefore, pcAfter, 3) {
				return 17
			}
			return 10
		case 5:
			if q == 0 {
				return 11
			}
			if p == 0 {
				return 17
			}
			return 0
		case 6:
			return 7
		default:
			return 11
		}
	}
}

func edCycles(op byte) int {
	x, y, z := op>>6, (op>>3)&7, op&7
	if x == 2 && z <= 3 && y >= 4 {
		return 16 // bloques; c.extra añade el tiempo de repetición
	}
	if x != 1 {
		return 8
	}
	switch z {
	case 0, 1:
		return 12
	case 2:
		return 15
	case 3:
		return 20
	case 4:
		return 8
	case 5:
		return 14
	case 6:
		return 8
	}
	switch y {
	case 0, 1, 2, 3:
		return 9
	case 4, 5:
		return 18
	}
	return 8
}

func usesHL(op byte) bool {
	x, y, z := op>>6, (op>>3)&7, op&7
	switch x {
	case 0:
		switch z {
		case 4, 5, 6:
			return y == 6
		}
		return false
	case 1:
		return y == 6 || z == 6
	case 2:
		return z == 6
	}
	return false
}

func idxCycles(b []byte) int {
	sub := b[1]
	if sub == 0xCB {
		return 23
	}
	base := mainCycles(sub, 0, 0)
	if usesHL(sub) {
		base += 12
	}
	return 4 + base
}
