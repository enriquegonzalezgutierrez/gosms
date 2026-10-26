package cpu

import (
	"encoding/gob"
	"fmt"
	"os"

	"gosms/pkg/psg"
	"gosms/pkg/vdp"
)

const (
	BankSize = 0x4000
)

// CPU implementa el Z80 de la Sega Master System.
type CPU struct {
	Tracing                        bool
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

	NMI                 bool // Botón PAUSE físico de la consola
	ROM                 []byte
	RAM                 [0x2000]byte
	CartRAM             [0x8000]byte // Hasta 32 KB de SRAM de cartucho
	RAMControl          byte         // Registro $FFFC del mapper Sega
	SRAMPath            string       // Ruta al archivo .sav
	SRAMDirty           bool
	Bank                [3]byte // registros del mapper: $FFFD, $FFFE, $FFFF
	page0, page1, page2 []byte
	isCodemasters       bool

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

func (c *CPU) updateBanks() {
	if len(c.ROM) == 0 {
		return
	}
	n := len(c.ROM) / BankSize
	if n == 0 {
		n = 1
	}
	b0 := (int(c.Bank[0]) % n) * BankSize
	b1 := (int(c.Bank[1]) % n) * BankSize
	b2 := (int(c.Bank[2]) % n) * BankSize
	c.page0 = c.ROM[b0 : b0+BankSize]
	c.page1 = c.ROM[b1 : b1+BankSize]
	c.page2 = c.ROM[b2 : b2+BankSize]
}

func isCodemastersROM(rom []byte) bool {
	if len(rom) < 0x8000 {
		return false
	}
	c1 := uint32(rom[0x7FE6]) | (uint32(rom[0x7FE7]) << 8)
	c2 := uint32(rom[0x7FE8]) | (uint32(rom[0x7FE9]) << 8)
	zeros := rom[0x7FEA] == 0 && rom[0x7FEB] == 0 && rom[0x7FEC] == 0 &&
		rom[0x7FED] == 0 && rom[0x7FEE] == 0 && rom[0x7FEF] == 0
	return zeros && rom[0x7FE0] == 0x10 && c1 != 0 && (c1+c2) == 0x10000
}

// NewCPU crea una CPU con una ROM y un VDP/PSG asociados.
func NewCPU(rom []byte) *CPU {
	if len(rom)%1024 == 512 {
		rom = rom[512:]
	}
	c := &CPU{
		SP:            0xDFF0,
		ROM:           rom,
		Bank:          [3]byte{0, 1, 2},
		ixName:        "HL",
		memName:       "(HL)",
		isCodemasters: isCodemastersROM(rom),
		VDP:           vdp.New(),
		PSG:           psg.New(44100),
		Pad1:          0xFF,
		Pad2:          0xFF,
	}
	c.updateBanks()
	return c
}

// ---------- Memoria ----------

func (c *CPU) ReadByte(addr uint16) byte {
	switch {
	case addr < 0x0400:
		if c.isCodemasters {
			return c.page0[addr]
		}
		return c.ROM[addr]
	case addr < 0x4000:
		return c.page0[addr]
	case addr < 0x8000:
		return c.page1[addr-0x4000]
	case addr < 0xC000:
		if c.RAMControl&0x08 != 0 {
			offset := int(addr - 0x8000)
			if c.RAMControl&0x04 != 0 {
				offset += 0x4000
			}
			return c.CartRAM[offset&0x7FFF]
		}
		return c.page2[addr-0x8000]
	default:
		return c.RAM[(addr-0xC000)&0x1FFF]
	}
}

func (c *CPU) WriteByte(addr uint16, val byte) {
	if WatchAddr != 0 && addr == WatchAddr && MemWatch != nil {
		MemWatch(addr, val, c)
	}

	if c.isCodemasters {
		if addr < 0xC000 {
			slot := addr / 0x4000
			n := len(c.ROM) / BankSize
			if n > 0 {
				c.Bank[slot] = byte(int(val) % n)
				c.updateBanks()
			}
			return
		}
		c.RAM[(addr-0xC000)&0x1FFF] = val
		return
	}

	// Escritura en SRAM de cartucho (000..)
	if addr >= 0x8000 && addr < 0xC000 {
		if c.RAMControl&0x08 != 0 {
			offset := int(addr - 0x8000)
			if c.RAMControl&0x04 != 0 {
				offset += 0x4000
			}
			c.CartRAM[offset&0x7FFF] = val
			c.SRAMDirty = true
			return
		}
	}

	if addr < 0xC000 {
		return // ROM de solo lectura
	}

	if addr == 0xFFFC { // Registro de control de SRAM del mapper Sega
		c.RAMControl = val
	}
	if addr >= 0xFFFD { // registros del mapper Sega para bancos ROM
		n := len(c.ROM) / BankSize
		if n > 0 {
			c.Bank[addr-0xFFFD] = byte(int(val) % n)
			c.updateBanks()
		}
	}
	c.RAM[(addr-0xC000)&0x1FFF] = val
}

func (c *CPU) LoadSRAM(path string) {
	c.SRAMPath = path
	if data, err := os.ReadFile(path); err == nil {
		copy(c.CartRAM[:], data)
	}
}

// SaveSRAM persiste en disco los datos de guardado del juego
func (c *CPU) SaveSRAM() {
	if c.SRAMDirty && c.SRAMPath != "" {
		_ = os.WriteFile(c.SRAMPath, c.CartRAM[:], 0644)
		c.SRAMDirty = false
	}
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
	switch {
	case port&0xC0 == 0x40:
		if port&1 == 0 {
			return c.VDP.VCounter()
		}
		return c.VDP.HCounter()
	case port&0xC0 == 0x80:
		if port&1 == 0 {
			return c.VDP.ReadData()
		}
		return c.VDP.ReadStatus()
	case port&0xC0 == 0xC0:
		if port&1 == 0 {
			return c.Pad1
		}
		return c.Pad2
	case port&0xC0 == 0x00:
		return 0xFF // Hardware occidental: puertos flotantes devuelven $FF
	}
	return 0xFF
}

func (c *CPU) WritePort(port, val byte) {
	switch {
	case port&0xC0 == 0x40: // Puertos $40..$7F van al PSG (SN76489)
		c.PSG.Write(val)
	case port&0xC0 == 0x80: // Puertos $80..$BF van al VDP
		if port&1 == 0 {
			c.VDP.WriteData(val)
		} else {
			c.VDP.WriteControl(val)
		}
	case port&0xC0 == 0x00:
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
	c.ix = nil
	c.Instr++

	// Interrupción No Enmascarable (NMI) - Botón PAUSE de la consola
	if c.NMI {
		c.NMI = false
		c.IFF2 = c.IFF1
		c.IFF1 = false
		if c.Halted {
			c.Halted = false
		}
		c.Push16(c.PC)
		c.PC = 0x0066
		c.Cycles += 11
		return "<NMI>", 11, true
	}

	// Aceptación de interrupciones (con retardo de una instrucción tras EI).
	if c.eiPending {
		c.eiPending = false
	} else if c.IRQ && c.IFF1 {
		c.IFF1, c.IFF2 = false, false
		c.IRQ = false
		c.Halted = false
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
	c.Tracing = true
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

// WriteState serializa el estado completo de la CPU
func (c *CPU) WriteState(w *os.File) error {
	var state = struct {
		PC, SP, IX, IY                 uint16
		A, F, B, C, D, E, H, L         byte
		A2, F2, B2, C2, D2, E2, H2, L2 byte
		I, R                           byte
		IFF1, IFF2                     bool
		IM                             byte
		Halted, eiPending, IRQ, NMI    bool
		Cycles, Instr                  uint64
		Bank                           [3]byte
		RAMControl                     byte
		RAM                            [0x2000]byte
		CartRAM                        [0x8000]byte
	}{
		PC: c.PC, SP: c.SP, IX: c.IX, IY: c.IY,
		A: c.A, F: c.F, B: c.B, C: c.C, D: c.D, E: c.E, H: c.H, L: c.L,
		A2: c.A2, F2: c.F2, B2: c.B2, C2: c.C2, D2: c.D2, E2: c.E2, H2: c.H2, L2: c.L2,
		I: c.I, R: c.R, IFF1: c.IFF1, IFF2: c.IFF2, IM: c.IM,
		Halted: c.Halted, eiPending: c.eiPending, IRQ: c.IRQ, NMI: c.NMI,
		Cycles: c.Cycles, Instr: c.Instr,
		Bank: c.Bank, RAMControl: c.RAMControl,
		RAM: c.RAM, CartRAM: c.CartRAM,
	}
	dec := gob.NewEncoder(w)
	return dec.Encode(&state)
}

// ReadState restaura el estado completo de la CPU
func (c *CPU) ReadState(r *os.File) error {
	var state struct {
		PC, SP, IX, IY                 uint16
		A, F, B, C, D, E, H, L         byte
		A2, F2, B2, C2, D2, E2, H2, L2 byte
		I, R                           byte
		IFF1, IFF2                     bool
		IM                             byte
		Halted, eiPending, IRQ, NMI    bool
		Cycles, Instr                  uint64
		Bank                           [3]byte
		RAMControl                     byte
		RAM                            [0x2000]byte
		CartRAM                        [0x8000]byte
	}
	dec := gob.NewDecoder(r)
	if err := dec.Decode(&state); err != nil {
		return err
	}
	c.PC, c.SP, c.IX, c.IY = state.PC, state.SP, state.IX, state.IY
	c.A, c.F, c.B, c.C, c.D, c.E, c.H, c.L = state.A, state.F, state.B, state.C, state.D, state.E, state.H, state.L
	c.A2, c.F2, c.B2, c.C2, c.D2, c.E2, c.H2, c.L2 = state.A2, state.F2, state.B2, state.C2, state.D2, state.E2, state.H2, state.L2
	c.I, c.R, c.IFF1, c.IFF2, c.IM = state.I, state.R, state.IFF1, state.IFF2, state.IM
	c.Halted, c.eiPending, c.IRQ, c.NMI = state.Halted, state.eiPending, state.IRQ, state.NMI
	c.Cycles, c.Instr = state.Cycles, state.Instr
	c.Bank, c.RAMControl = state.Bank, state.RAMControl
	c.RAM, c.CartRAM = state.RAM, state.CartRAM
	c.updateBanks()
	return nil
}
