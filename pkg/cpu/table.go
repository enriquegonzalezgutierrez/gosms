package cpu

import "fmt"

func (c *CPU) sprintf(format string, a ...any) string {
	if !c.Tracing {
		return ""
	}
	return fmt.Sprintf(format, a...)
}

// Decodificador completo del Z80 (instrucciones documentadas y las
// indocumentadas más comunes). Se decodifica por campos de bits:
//   op = xx yyy zzz   (p = y>>1, q = y&1)

const (
	FC  = 0x01
	FN  = 0x02
	FPV = 0x04
	FX  = 0x08
	FH  = 0x10
	FY  = 0x20
	FZ  = 0x40
	FS  = 0x80
)

var (
	regNames   = [8]string{"B", "C", "D", "E", "H", "L", "(HL)", "A"}
	rpNames    = [4]string{"BC", "DE", "HL", "SP"}
	ccNames    = [8]string{"NZ", "Z", "NC", "C", "PO", "PE", "P", "M"}
	aluNames   = [8]string{"ADD A,", "ADC A,", "SUB ", "SBC A,", "AND ", "XOR ", "OR ", "CP "}
	rotNames   = [8]string{"RLC", "RRC", "RL", "RR", "SLA", "SRA", "SLL", "SRL"}
	blockNames = [4][4]string{
		{"LDI", "LDD", "LDIR", "LDDR"},
		{"CPI", "CPD", "CPIR", "CPDR"},
		{"INI", "IND", "INIR", "INDR"},
		{"OUTI", "OUTD", "OTIR", "OTDR"},
	}
)

// ---------- Helpers de flags / ALU ----------

func szf(r byte) byte {
	f := r & (FX | FY)
	if r == 0 {
		f |= FZ
	}
	return f | r&FS
}

func parity(v byte) bool {
	n := 0
	for ; v != 0; v >>= 1 {
		n += int(v & 1)
	}
	return n%2 == 0
}

func (c *CPU) add8(b, carry byte) byte {
	a := c.A
	r := uint16(a) + uint16(b) + uint16(carry)
	res := byte(r)
	f := szf(res)
	if (a&0xF)+(b&0xF)+carry > 0xF {
		f |= FH
	}
	if (a^res)&(b^res)&0x80 != 0 {
		f |= FPV
	}
	if r > 0xFF {
		f |= FC
	}
	c.F = f
	return res
}

func (c *CPU) sub8(b, carry byte) byte { // SUB, SBC y CP
	a := c.A
	r := int(a) - int(b) - int(carry)
	res := byte(r)
	f := szf(res) | FN
	if int(a&0xF)-int(b&0xF)-int(carry) < 0 {
		f |= FH
	}
	if (a^b)&(a^res)&0x80 != 0 {
		f |= FPV
	}
	if r < 0 {
		f |= FC
	}
	c.F = f
	return res
}

func (c *CPU) logic(r byte, h byte) {
	c.A = r
	c.F = szf(r) | h
	if parity(r) {
		c.F |= FPV
	}
}

func (c *CPU) alu(op byte, v byte) {
	switch op {
	case 0:
		c.A = c.add8(v, 0)
	case 1:
		c.A = c.add8(v, c.F&FC)
	case 2:
		c.A = c.sub8(v, 0)
	case 3:
		c.A = c.sub8(v, c.F&FC)
	case 4:
		c.logic(c.A&v, FH)
	case 5:
		c.logic(c.A^v, 0)
	case 6:
		c.logic(c.A|v, 0)
	case 7: // CP: solo flags; bits 3 y 5 vienen del operando
		c.sub8(v, 0)
		c.F = c.F&^(FX|FY) | v&(FX|FY)
	}
}

func (c *CPU) inc8(v byte) byte {
	r := v + 1
	f := c.F&FC | szf(r)
	if v&0xF == 0xF {
		f |= FH
	}
	if v == 0x7F {
		f |= FPV
	}
	c.F = f
	return r
}

func (c *CPU) dec8(v byte) byte {
	r := v - 1
	f := c.F&FC | szf(r) | FN
	if v&0xF == 0 {
		f |= FH
	}
	if v == 0x80 {
		f |= FPV
	}
	c.F = f
	return r
}

func (c *CPU) add16(a, b uint16) uint16 {
	r := uint32(a) + uint32(b)
	f := c.F & (FS | FZ | FPV)
	f |= byte(r>>8) & (FX | FY)
	if (a&0xFFF)+(b&0xFFF) > 0xFFF {
		f |= FH
	}
	if r > 0xFFFF {
		f |= FC
	}
	c.F = f
	return uint16(r)
}

func (c *CPU) adc16(a, b uint16) uint16 {
	cy := uint16(c.F & FC)
	r := uint32(a) + uint32(b) + uint32(cy)
	res := uint16(r)
	f := byte(res>>8) & (FS | FX | FY)
	if res == 0 {
		f |= FZ
	}
	if (a&0xFFF)+(b&0xFFF)+cy > 0xFFF {
		f |= FH
	}
	if (a^res)&(b^res)&0x8000 != 0 {
		f |= FPV
	}
	if r > 0xFFFF {
		f |= FC
	}
	c.F = f
	return res
}

func (c *CPU) sbc16(a, b uint16) uint16 {
	cy := int32(c.F & FC)
	r := int32(a) - int32(b) - cy
	res := uint16(r)
	f := byte(res>>8)&(FS|FX|FY) | FN
	if res == 0 {
		f |= FZ
	}
	if int32(a&0xFFF)-int32(b&0xFFF)-cy < 0 {
		f |= FH
	}
	if (a^b)&(a^res)&0x8000 != 0 {
		f |= FPV
	}
	if r < 0 {
		f |= FC
	}
	c.F = f
	return res
}

func (c *CPU) accFlags(co byte) { // RLCA/RRCA/RLA/RRA
	c.F = c.F&(FS|FZ|FPV) | c.A&(FX|FY) | co
}

func (c *CPU) daa() {
	a := c.A
	f := c.F
	var corr byte
	carry := f&FC != 0
	if f&FH != 0 || a&0xF > 9 {
		corr |= 0x06
	}
	if carry || a > 0x99 {
		corr |= 0x60
		carry = true
	}
	var r byte
	var h bool
	if f&FN != 0 {
		r = a - corr
		h = f&FH != 0 && a&0xF < 6
	} else {
		r = a + corr
		h = a&0xF > 9
	}
	nf := szf(r) | f&FN
	if parity(r) {
		nf |= FPV
	}
	if h {
		nf |= FH
	}
	if carry {
		nf |= FC
	}
	c.A, c.F = r, nf
}

func (c *CPU) rot(y, v byte) byte {
	var r, co byte
	switch y {
	case 0: // RLC
		co = v >> 7
		r = v<<1 | co
	case 1: // RRC
		co = v & 1
		r = v>>1 | co<<7
	case 2: // RL
		co = v >> 7
		r = v<<1 | c.F&FC
	case 3: // RR
		co = v & 1
		r = v>>1 | (c.F&FC)<<7
	case 4: // SLA
		co = v >> 7
		r = v << 1
	case 5: // SRA
		co = v & 1
		r = v>>1 | v&0x80
	case 6: // SLL (indocumentada)
		co = v >> 7
		r = v<<1 | 1
	default: // SRL
		co = v & 1
		r = v >> 1
	}
	c.F = szf(r) | co
	if parity(r) {
		c.F |= FPV
	}
	return r
}

func (c *CPU) bit(y, v byte) {
	f := c.F&FC | FH
	r := v & (1 << y)
	if r == 0 {
		f |= FZ | FPV
	}
	f |= r & FS
	f |= v & (FX | FY)
	c.F = f
}

func (c *CPU) cond(cc byte) bool {
	switch cc {
	case 0:
		return c.F&FZ == 0
	case 1:
		return c.F&FZ != 0
	case 2:
		return c.F&FC == 0
	case 3:
		return c.F&FC != 0
	case 4:
		return c.F&FPV == 0
	case 5:
		return c.F&FPV != 0
	case 6:
		return c.F&FS == 0
	}
	return c.F&FS != 0
}

// ---------- Acceso a registros (con soporte de IX/IY) ----------

// hlAddr devuelve la dirección del operando (HL) o (IX+d). Debe llamarse una
// sola vez por instrucción, antes de leer cualquier operando inmediato.
func (c *CPU) hlAddr() uint16 {
	if c.ix == nil {
		c.memName = "(HL)"
		return c.hl()
	}
	d := int8(c.fetch8())
	c.memName = c.sprintf("(%s%+d)", c.ixName, d)
	return *c.ix + uint16(int16(d))
}

// plain=true: usa H/L reales aunque haya prefijo DD/FD.
func (c *CPU) getR(i byte, addr uint16, plain bool) byte {
	ix := c.ix
	if plain {
		ix = nil
	}
	switch i {
	case 0:
		return c.B
	case 1:
		return c.C
	case 2:
		return c.D
	case 3:
		return c.E
	case 4:
		if ix != nil {
			return byte(*ix >> 8)
		}
		return c.H
	case 5:
		if ix != nil {
			return byte(*ix)
		}
		return c.L
	case 6:
		return c.ReadByte(addr)
	}
	return c.A
}

func (c *CPU) setR(i byte, addr uint16, plain bool, v byte) {
	ix := c.ix
	if plain {
		ix = nil
	}
	switch i {
	case 0:
		c.B = v
	case 1:
		c.C = v
	case 2:
		c.D = v
	case 3:
		c.E = v
	case 4:
		if ix != nil {
			*ix = uint16(v)<<8 | *ix&0x00FF
		} else {
			c.H = v
		}
	case 5:
		if ix != nil {
			*ix = *ix&0xFF00 | uint16(v)
		} else {
			c.L = v
		}
	case 6:
		c.WriteByte(addr, v)
	case 7:
		c.A = v
	}
}

func (c *CPU) rName(i byte, plain bool) string {
	if i == 6 {
		return c.memName
	}
	if !plain && c.ix != nil {
		if i == 4 {
			return c.ixName + "H"
		}
		if i == 5 {
			return c.ixName + "L"
		}
	}
	return regNames[i]
}

func (c *CPU) rpName(p byte) string {
	if p == 2 {
		return c.ixName
	}
	return rpNames[p]
}

func (c *CPU) rp2Name(p byte) string {
	switch p {
	case 2:
		return c.ixName
	case 3:
		return "AF"
	}
	return rpNames[p]
}

func (c *CPU) getRP(p byte) uint16 {
	switch p {
	case 0:
		return c.bc()
	case 1:
		return c.de()
	case 2:
		if c.ix != nil {
			return *c.ix
		}
		return c.hl()
	}
	return c.SP
}

func (c *CPU) setRP(p byte, v uint16) {
	switch p {
	case 0:
		c.setBC(v)
	case 1:
		c.setDE(v)
	case 2:
		if c.ix != nil {
			*c.ix = v
		} else {
			c.setHL(v)
		}
	default:
		c.SP = v
	}
}

func (c *CPU) getRP2(p byte) uint16 {
	if p == 3 {
		return uint16(c.A)<<8 | uint16(c.F)
	}
	return c.getRP(p)
}

func (c *CPU) setRP2(p byte, v uint16) {
	if p == 3 {
		c.A, c.F = byte(v>>8), byte(v)
		return
	}
	c.setRP(p, v)
}

// ---------- Paso de ejecución ----------

func flagStr(f byte) string {
	s := []byte("........")
	for i, ch := range []byte("SZ5H3PNC") {
		if f&(0x80>>uint(i)) != 0 {
			s[i] = ch
		}
	}
	return string(s)
}

func (c *CPU) relJump() uint16 {
	d := int8(c.fetch8())
	return c.PC + uint16(int16(d))
}

func (c *CPU) execMain(op byte) (string, bool) {
	x, y, z := op>>6, (op>>3)&7, op&7
	p, q := y>>1, y&1

	switch x {
	case 0:
		switch z {
		case 0:
			switch y {
			case 0:
				return "NOP", true
			case 1:
				c.A, c.A2 = c.A2, c.A
				c.F, c.F2 = c.F2, c.F
				return "EX AF,AF'", true
			case 2:
				t := c.relJump()
				c.B--
				if c.B != 0 {
					c.PC = t
				}
				return c.sprintf("DJNZ $%04X", t), true
			case 3:
				t := c.relJump()
				c.PC = t
				return c.sprintf("JR $%04X", t), true
			default:
				t := c.relJump()
				if c.cond(y - 4) {
					c.PC = t
				}
				return c.sprintf("JR %s,$%04X", ccNames[y-4], t), true
			}
		case 1:
			if q == 0 {
				n := c.fetch16()
				c.setRP(p, n)
				return c.sprintf("LD %s,$%04X", c.rpName(p), n), true
			}
			hl, rr := c.getRP(2), c.getRP(p)
			c.setRP(2, c.add16(hl, rr))
			return c.sprintf("ADD %s,%s", c.rpName(2), c.rpName(p)), true
		case 2:
			switch p {
			case 0, 1:
				addr := c.getRP(p)
				if q == 0 {
					c.WriteByte(addr, c.A)
					return c.sprintf("LD (%s),A", rpNames[p]), true
				}
				c.A = c.ReadByte(addr)
				return c.sprintf("LD A,(%s)", rpNames[p]), true
			case 2:
				n := c.fetch16()
				if q == 0 {
					v := c.getRP(2)
					c.WriteByte(n, byte(v))
					c.WriteByte(n+1, byte(v>>8))
					return c.sprintf("LD ($%04X),%s", n, c.rpName(2)), true
				}
				lo, hi := c.ReadByte(n), c.ReadByte(n+1)
				c.setRP(2, uint16(hi)<<8|uint16(lo))
				return c.sprintf("LD %s,($%04X)", c.rpName(2), n), true
			default:
				n := c.fetch16()
				if q == 0 {
					c.WriteByte(n, c.A)
					return c.sprintf("LD ($%04X),A", n), true
				}
				c.A = c.ReadByte(n)
				return c.sprintf("LD A,($%04X)", n), true
			}
		case 3:
			if q == 0 {
				c.setRP(p, c.getRP(p)+1)
				return "INC " + c.rpName(p), true
			}
			c.setRP(p, c.getRP(p)-1)
			return "DEC " + c.rpName(p), true
		case 4, 5:
			var addr uint16
			if y == 6 {
				addr = c.hlAddr()
			}
			v := c.getR(y, addr, false)
			if z == 4 {
				c.setR(y, addr, false, c.inc8(v))
				return "INC " + c.rName(y, false), true
			}
			c.setR(y, addr, false, c.dec8(v))
			return "DEC " + c.rName(y, false), true
		case 6:
			var addr uint16
			if y == 6 {
				addr = c.hlAddr()
			}
			n := c.fetch8()
			c.setR(y, addr, false, n)
			return c.sprintf("LD %s,$%02X", c.rName(y, false), n), true
		case 7:
			switch y {
			case 0:
				co := c.A >> 7
				c.A = c.A<<1 | co
				c.accFlags(co)
				return "RLCA", true
			case 1:
				co := c.A & 1
				c.A = c.A>>1 | co<<7
				c.accFlags(co)
				return "RRCA", true
			case 2:
				co := c.A >> 7
				c.A = c.A<<1 | c.F&FC
				c.accFlags(co)
				return "RLA", true
			case 3:
				co := c.A & 1
				c.A = c.A>>1 | (c.F&FC)<<7
				c.accFlags(co)
				return "RRA", true
			case 4:
				c.daa()
				return "DAA", true
			case 5:
				c.A = ^c.A
				c.F = c.F&(FS|FZ|FPV|FC) | FH | FN | c.A&(FX|FY)
				return "CPL", true
			case 6:
				c.F = c.F&(FS|FZ|FPV) | FC | c.A&(FX|FY)
				return "SCF", true
			default:
				f := c.F&(FS|FZ|FPV) | c.A&(FX|FY)
				if c.F&FC != 0 {
					f |= FH
				} else {
					f |= FC
				}
				c.F = f
				return "CCF", true
			}
		}

	case 1:
		if op == 0x76 {
			c.Halted = true
			return "HALT", true
		}
		var addr uint16
		plain := y == 6 || z == 6
		if plain {
			addr = c.hlAddr()
		}
		c.setR(y, addr, plain, c.getR(z, addr, plain))
		return c.sprintf("LD %s,%s", c.rName(y, plain), c.rName(z, plain)), true

	case 2:
		var addr uint16
		if z == 6 {
			addr = c.hlAddr()
		}
		c.alu(y, c.getR(z, addr, false))
		return aluNames[y] + c.rName(z, false), true

	case 3:
		switch z {
		case 0:
			if c.cond(y) {
				c.PC = c.Pop16()
			}
			return "RET " + ccNames[y], true
		case 1:
			if q == 0 {
				c.setRP2(p, c.Pop16())
				return "POP " + c.rp2Name(p), true
			}
			switch p {
			case 0:
				c.PC = c.Pop16()
				return "RET", true
			case 1: // EXX
				c.B, c.B2 = c.B2, c.B
				c.C, c.C2 = c.C2, c.C
				c.D, c.D2 = c.D2, c.D
				c.E, c.E2 = c.E2, c.E
				c.H, c.H2 = c.H2, c.H
				c.L, c.L2 = c.L2, c.L
				return "EXX", true
			case 2:
				c.PC = c.getRP(2)
				return c.sprintf("JP (%s)", c.ixName), true
			default:
				c.SP = c.getRP(2)
				return c.sprintf("LD SP,%s", c.ixName), true
			}
		case 2:
			n := c.fetch16()
			if c.cond(y) {
				c.PC = n
			}
			return c.sprintf("JP %s,$%04X", ccNames[y], n), true
		case 3:
			switch y {
			case 0:
				n := c.fetch16()
				c.PC = n
				return c.sprintf("JP $%04X", n), true
			case 1:
				return c.execCB()
			case 2:
				n := c.fetch8()
				c.WritePort(n, c.A)
				return c.sprintf("OUT ($%02X),A", n), true
			case 3:
				n := c.fetch8()
				c.A = c.ReadPort(n)
				return c.sprintf("IN A,($%02X)", n), true
			case 4:
				lo, hi := c.ReadByte(c.SP), c.ReadByte(c.SP+1)
				v := c.getRP(2)
				c.WriteByte(c.SP, byte(v))
				c.WriteByte(c.SP+1, byte(v>>8))
				c.setRP(2, uint16(hi)<<8|uint16(lo))
				return c.sprintf("EX (SP),%s", c.ixName), true
			case 5:
				c.D, c.H = c.H, c.D
				c.E, c.L = c.L, c.E
				return "EX DE,HL", true
			case 6:
				c.IFF1, c.IFF2 = false, false
				return "DI", true
			default:
				c.IFF1, c.IFF2 = true, true
				c.eiPending = true
				return "EI", true
			}
		case 4:
			n := c.fetch16()
			if c.cond(y) {
				c.Push16(c.PC)
				c.PC = n
			}
			return c.sprintf("CALL %s,$%04X", ccNames[y], n), true
		case 5:
			if q == 0 {
				c.Push16(c.getRP2(p))
				return "PUSH " + c.rp2Name(p), true
			}
			switch p {
			case 0:
				n := c.fetch16()
				c.Push16(c.PC)
				c.PC = n
				return c.sprintf("CALL $%04X", n), true
			case 1:
				return c.execIndex(&c.IX, "IX")
			case 2:
				return c.execED()
			default:
				return c.execIndex(&c.IY, "IY")
			}
		case 6:
			n := c.fetch8()
			c.alu(y, n)
			return c.sprintf("%s$%02X", aluNames[y], n), true
		case 7:
			c.Push16(c.PC)
			c.PC = uint16(y) * 8
			return c.sprintf("RST $%02X", uint16(y)*8), true
		}
	}
	return c.sprintf("OPCODE $%02X NO IMPLEMENTADO", op), false
}

// ---------- Prefijo CB ----------

func (c *CPU) execCB() (string, bool) {
	op := c.fetch8()
	x, y, z := op>>6, (op>>3)&7, op&7
	var addr uint16
	if z == 6 {
		addr = c.hlAddr()
	}
	v := c.getR(z, addr, true)
	switch x {
	case 0:
		c.setR(z, addr, true, c.rot(y, v))
		return rotNames[y] + " " + c.rName(z, true), true
	case 1:
		c.bit(y, v)
		return c.sprintf("BIT %d,%s", y, c.rName(z, true)), true
	case 2:
		c.setR(z, addr, true, v&^(1<<y))
		return c.sprintf("RES %d,%s", y, c.rName(z, true)), true
	}
	c.setR(z, addr, true, v|(1<<y))
	return c.sprintf("SET %d,%s", y, c.rName(z, true)), true
}

// ---------- Prefijos DD / FD ----------

func (c *CPU) execIndex(ix *uint16, name string) (string, bool) {
	op := c.fetch8()
	if op == 0xDD || op == 0xFD || op == 0xED {
		// Prefijo sin efecto: se ejecuta el siguiente como instrucción normal
		c.PC--
		c.opLen--
		return name + " (prefijo ignorado)", true
	}
	c.ix, c.ixName = ix, name
	if op != 0xCB {
		return c.execMain(op)
	}
	// DD CB d op
	d := int8(c.fetch8())
	op2 := c.fetch8()
	addr := *ix + uint16(int16(d))
	mem := c.sprintf("(%s%+d)", name, d)
	x, y, z := op2>>6, (op2>>3)&7, op2&7
	v := c.ReadByte(addr)
	switch x {
	case 0:
		r := c.rot(y, v)
		c.WriteByte(addr, r)
		if z != 6 {
			c.setR(z, 0, true, r)
		}
		return rotNames[y] + " " + mem, true
	case 1:
		c.bit(y, v)
		return c.sprintf("BIT %d,%s", y, mem), true
	case 2:
		r := v &^ (1 << y)
		c.WriteByte(addr, r)
		if z != 6 {
			c.setR(z, 0, true, r)
		}
		return c.sprintf("RES %d,%s", y, mem), true
	}
	r := v | (1 << y)
	c.WriteByte(addr, r)
	if z != 6 {
		c.setR(z, 0, true, r)
	}
	return c.sprintf("SET %d,%s", y, mem), true
}

// ---------- Prefijo ED ----------

func (c *CPU) execED() (string, bool) {
	op := c.fetch8()
	x, y, z := op>>6, (op>>3)&7, op&7
	p, q := y>>1, y&1

	if x == 2 && z <= 3 && y >= 4 {
		return c.execBlock(y, z), true
	}
	if x != 1 {
		return c.sprintf("NOP* (ED %02X)", op), true
	}

	switch z {
	case 0: // IN r,(C)
		v := c.ReadPort(c.C)
		name := "IN F,(C)"
		if y != 6 {
			c.setR(y, 0, true, v)
			name = c.sprintf("IN %s,(C)", regNames[y])
		}
		c.F = c.F&FC | szf(v)
		if parity(v) {
			c.F |= FPV
		}
		return name, true
	case 1: // OUT (C),r
		var v byte
		name := "OUT (C),0"
		if y != 6 {
			v = c.getR(y, 0, true)
			name = c.sprintf("OUT (C),%s", regNames[y])
		}
		c.WritePort(c.C, v)
		return name, true
	case 2:
		hl, rr := c.hl(), c.getRP(p)
		if q == 0 {
			c.setHL(c.sbc16(hl, rr))
			return "SBC HL," + rpNames[p], true
		}
		c.setHL(c.adc16(hl, rr))
		return "ADC HL," + rpNames[p], true
	case 3:
		n := c.fetch16()
		if q == 0 {
			v := c.getRP(p)
			c.WriteByte(n, byte(v))
			c.WriteByte(n+1, byte(v>>8))
			return c.sprintf("LD ($%04X),%s", n, rpNames[p]), true
		}
		lo, hi := c.ReadByte(n), c.ReadByte(n+1)
		c.setRP(p, uint16(hi)<<8|uint16(lo))
		return c.sprintf("LD %s,($%04X)", rpNames[p], n), true
	case 4:
		v := c.A
		c.A = 0
		c.A = c.sub8(v, 0)
		return "NEG", true
	case 5:
		c.PC = c.Pop16()
		c.IFF1 = c.IFF2
		if y == 1 {
			return "RETI", true
		}
		return "RETN", true
	case 6:
		modes := [8]byte{0, 0, 1, 2, 0, 0, 1, 2}
		c.IM = modes[y]
		return c.sprintf("IM %d", c.IM), true
	}
	// z == 7
	switch y {
	case 0:
		c.I = c.A
		return "LD I,A", true
	case 1:
		c.R = c.A
		return "LD R,A", true
	case 2, 3:
		name := "LD A,I"
		if y == 2 {
			c.A = c.I
		} else {
			c.A = c.R
			name = "LD A,R"
		}
		f := c.F&FC | szf(c.A)
		if c.IFF2 {
			f |= FPV
		}
		c.F = f
		return name, true
	case 4, 5:
		hl := c.hl()
		m := c.ReadByte(hl)
		t := c.A & 0xF
		name := "RRD"
		if y == 4 {
			c.A = c.A&0xF0 | m&0xF
			m = t<<4 | m>>4
		} else {
			c.A = c.A&0xF0 | m>>4
			m = m<<4 | t
			name = "RLD"
		}
		c.WriteByte(hl, m)
		c.F = c.F&FC | szf(c.A)
		if parity(c.A) {
			c.F |= FPV
		}
		return name, true
	}
	return "NOP*", true
}

// LDI/LDD/LDIR/LDDR, CPI.., INI.., OUTI..: cada iteración se ejecuta en un
// paso. Las variantes con repetición rebobinan el PC para repetir, dejando
// que las interrupciones se atiendan entre iteraciones.
func (c *CPU) execBlock(y, z byte) string {
	dec := y&1 == 1
	rep := y >= 6
	step := func(v uint16) uint16 {
		if dec {
			return v - 1
		}
		return v + 1
	}
	done := true
	switch z {
	case 0: // LD
		v := c.ReadByte(c.hl())
		c.WriteByte(c.de(), v)
		c.setHL(step(c.hl()))
		c.setDE(step(c.de()))
		c.setBC(c.bc() - 1)
		f := c.F & (FS | FZ | FC)
		if c.bc() != 0 {
			f |= FPV
			done = false
		}
		c.F = f
	case 1: // CP
		v := c.ReadByte(c.hl())
		res := c.A - v
		c.setHL(step(c.hl()))
		c.setBC(c.bc() - 1)
		f := c.F&FC | FN | res&FS
		if res == 0 {
			f |= FZ
		}
		if c.A&0xF < v&0xF {
			f |= FH
		}
		if c.bc() != 0 {
			f |= FPV
			if res != 0 {
				done = false
			}
		}
		c.F = f
	case 2: // IN
		c.WriteByte(c.hl(), c.ReadPort(c.C))
		c.setHL(step(c.hl()))
		c.B--
		f := c.F&FC | FN
		if c.B == 0 {
			f |= FZ
		} else {
			done = false
		}
		c.F = f
	case 3: // OUT
		c.B--
		c.WritePort(c.C, c.ReadByte(c.hl()))
		c.setHL(step(c.hl()))
		f := c.F&FC | FN
		if c.B == 0 {
			f |= FZ
		} else {
			done = false
		}
		c.F = f
	}
	if rep && !done {
		c.PC -= 2 // repite esta misma instrucción
		c.extra += 5
	}
	return blockNames[z][y-4]
}
