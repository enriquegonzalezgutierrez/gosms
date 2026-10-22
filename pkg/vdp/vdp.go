// Package vdp implementa el VDP de la Sega Master System (modo 4).
package vdp

const (
	ScreenW       = 256
	ScreenH       = 192
	TotalLines    = 262
	CyclesPerLine = 228 // ciclos de CPU por scanline (NTSC, ~59.92 Hz)
	VRAMSize      = 0x4000
)

// debugRegs activa el volcado de escrituras de registro (depuración).
var debugRegs = false

// SetDebugRegs activa o desactiva la traza de escrituras de registro.
func SetDebugRegs(b bool) { debugRegs = b }

// noSprites desactiva el dibujado de sprites (depuración).
var noSprites = false

// SetSpritesEnabled activa o desactiva el dibujado de sprites.
func SetSpritesEnabled(b bool) { noSprites = !b }

// DebugWrites cuenta las escrituras a VRAM/CRAM por dirección (depuración).
var DebugWrites [VRAMSize]uint32
var writeDebug = false

// SetWriteDebug activa el contador de escrituras por dirección.
func SetWriteDebug(b bool) {
	writeDebug = b
	if b {
		for i := range DebugWrites {
			DebugWrites[i] = 0
		}
	}
}

// WriteRec es una escritura a VRAM registrada para depuración.
type WriteRec struct {
	Addr uint16
	Val  byte
	PC   uint16
	A    byte
	HL   uint16
}

// DebugLog guarda la secuencia de escrituras a un rango de VRAM.
var DebugLog []WriteRec

// ContextHook devuelve PC, A y HL actuales de la CPU (depuración).
var ContextHook func() (uint16, byte, uint16)

var (
	logStart, logEnd uint16
	logOn            bool
)

// SetWriteLog activa el registro de escrituras dentro de [start, end).
func SetWriteLog(start, end uint16) {
	logStart, logEnd = start, end
	DebugLog = DebugLog[:0]
	logOn = true
}

// StopWriteLog detiene el registro de escrituras.
func StopWriteLog() { logOn = false }

// VDP modela el chip de vídeo.
type VDP struct {
	VRAM [VRAMSize]byte
	CRAM [32]byte
	Reg  [16]byte

	Status byte

	addr    uint16
	code    byte
	latch   bool
	low     byte
	readBuf byte

	line        int
	lineCounter int
	vScroll     byte
	irq         bool

	// Frame es el framebuffer RGBA (256x192) listo para volcar.
	Frame [ScreenW * ScreenH * 4]byte
}

// New crea un VDP con los valores de arranque del BIOS de la consola.
func New() *VDP {
	v := &VDP{}
	v.Reg[0] = 0x36
	v.Reg[1] = 0xA0
	v.Reg[2] = 0xFF
	v.Reg[3] = 0xFF
	v.Reg[4] = 0xFF
	v.Reg[5] = 0xFF
	v.Reg[6] = 0xFB
	v.Reg[7] = 0x00
	v.Reg[8] = 0x00
	v.Reg[9] = 0x00
	v.Reg[10] = 0xFF
	v.lineCounter = int(v.Reg[10])
	return v
}

// ---------- Puertos ----------

// ReadData lee el puerto de datos ($BE).
func (v *VDP) ReadData() byte {
	v.latch = false
	r := v.readBuf
	v.readBuf = v.VRAM[v.addr&0x3FFF]
	v.addr = (v.addr + 1) & 0x3FFF
	return r
}

// ReadStatus lee el puerto de estado/control ($BF). Limpia los flags.
func (v *VDP) ReadStatus() byte {
	v.latch = false
	r := v.Status
	v.Status &= 0x1F
	v.irq = false
	return r
}

// WriteData escribe el puerto de datos ($BE).
func (v *VDP) WriteData(val byte) {
	v.latch = false
	a := v.addr & 0x3FFF
	if writeDebug {
		DebugWrites[a]++
	}
	if logOn && a >= logStart && a < logEnd {
		var pc uint16
		var ac byte
		var hl uint16
		if ContextHook != nil {
			pc, ac, hl = ContextHook()
		}
		DebugLog = append(DebugLog, WriteRec{Addr: a, Val: val, PC: pc, A: ac, HL: hl})
	}
	if v.code == 3 {
		v.CRAM[v.addr&0x1F] = val
	} else {
		v.VRAM[v.addr&0x3FFF] = val
	}
	v.readBuf = val
	v.addr = (v.addr + 1) & 0x3FFF
}

// WriteControl escribe el puerto de control ($BF). Acepta la palabra de
// control en dos bytes (primero el bajo, luego el alto).
func (v *VDP) WriteControl(val byte) {
	if !v.latch {
		v.low = val
		v.latch = true
		return
	}
	v.latch = false
	v.code = val >> 6
	if v.code == 2 { // escritura de registro
		reg := val & 0x0F
		if reg < 16 {
			if debugRegs {
				println("VDP reg", reg, "=", v.low)
			}
			v.Reg[reg] = v.low
			if reg == 10 {
				v.lineCounter = int(v.low)
			}
		}
		return
	}
	v.addr = uint16(val&0x3F)<<8 | uint16(v.low)
	if v.code == 0 { // preparar lectura
		v.readBuf = v.VRAM[v.addr&0x3FFF]
		v.addr = (v.addr + 1) & 0x3FFF
	}
}

// VCounter devuelve el contador vertical ($7E).
func (v *VDP) VCounter() byte { return byte(v.line) }

// HCounter devuelve el contador horizontal ($7F). Aproximado.
func (v *VDP) HCounter() byte { return 0 }

// IRQ indica si hay una interrupción pendiente.
func (v *VDP) IRQ() bool { return v.irq }

// ClearIRQ marca la interrupción como atendida.
func (v *VDP) ClearIRQ() { v.irq = false }

// ---------- Temporización ----------

// SetLine se llama al comienzo de cada scanline.
func (v *VDP) SetLine(line int) {
	v.line = line

	if line == ScreenH { // comienza el VBlank
		v.Status |= 0x80
		v.vScroll = v.Reg[9] // el scroll vertical se fija en el VBlank
	}

	// Contador de interrupción de línea (activo y primera línea de blanking).
	if line <= ScreenH {
		if v.lineCounter == 0 {
			v.lineCounter = int(v.Reg[10])
			if v.Reg[0]&0x10 != 0 {
				v.irq = true
			}
		} else {
			v.lineCounter--
		}
	}

	if v.Reg[1]&0x20 != 0 && v.Status&0x80 != 0 {
		v.irq = true
	}
}

// ---------- Render ----------

func (v *VDP) color(idx int) uint32 {
	c := v.CRAM[idx&0x1F]
	r := uint32(c&3) * 85
	g := uint32((c>>2)&3) * 85
	b := uint32((c>>4)&3) * 85
	return r<<16 | g<<8 | b
}

func (v *VDP) setPixel(line, x int, rgb uint32) {
	if line < 0 || line >= ScreenH || x < 0 || x >= ScreenW {
		return
	}
	o := (line*ScreenW + x) * 4
	v.Frame[o] = byte(rgb >> 16)
	v.Frame[o+1] = byte(rgb >> 8)
	v.Frame[o+2] = byte(rgb)
	v.Frame[o+3] = 0xFF
}

// RenderLine dibuja la línea indicada en el framebuffer.
func (v *VDP) RenderLine(line int) {
	backdrop := v.color(16 + int(v.Reg[7]&0x0F))
	if v.Reg[1]&0x40 == 0 { // pantalla desactivada
		for x := 0; x < ScreenW; x++ {
			v.setPixel(line, x, backdrop)
		}
		return
	}
	scrollX := int(v.Reg[8])
	scrollY := int(v.vScroll)
	nameBase := int(v.Reg[2]&0x0E) << 10
	blankLeft := v.Reg[0]&0x20 != 0

	var bgPri [ScreenW]bool
	var palRGB [32]uint32
	for i := 0; i < 32; i++ {
		palRGB[i] = v.color(i)
	}

	lineOffset := line * ScreenW * 4
	lastCol := -1
	var b0, b1, b2, b3 byte
	var hflip, prio bool
	var po int

	// ---- Fondo ----
	for x := 0; x < ScreenW; x++ {
		pixOff := lineOffset + x*4
		if blankLeft && x < 8 {
			v.Frame[pixOff] = byte(backdrop >> 16)
			v.Frame[pixOff+1] = byte(backdrop >> 8)
			v.Frame[pixOff+2] = byte(backdrop)
			v.Frame[pixOff+3] = 0xFF
			continue
		}

		bgx := x - scrollX
		bgy := line + scrollY
		col := (bgx >> 3) & 0x1F
		fx := bgx & 7

		// Solo leemos el descriptor de la baldosa y los planos cuando cambia la columna (cada 8 px)
		if col != lastCol {
			lastCol = col
			row := (bgy >> 3) % 28
			fy := bgy & 7

			off := (nameBase + row*64 + col*2) & 0x3FFF
			entry := uint16(v.VRAM[(off+1)&0x3FFF])<<8 | uint16(v.VRAM[off])
			tile := int(entry & 0x1FF)
			hflip = entry&0x200 != 0
			vflip := entry&0x400 != 0
			pal := entry&0x800 != 0
			prio = entry&0x1000 != 0

			py := fy
			if vflip {
				py = 7 - fy
			}
			pat := (tile*32 + py*4) & 0x3FFF
			b0 = v.VRAM[pat]
			b1 = v.VRAM[(pat+1)&0x3FFF]
			b2 = v.VRAM[(pat+2)&0x3FFF]
			b3 = v.VRAM[(pat+3)&0x3FFF]

			po = 0
			if pal {
				po = 16
			}
		}

		px := fx
		if hflip {
			px = 7 - fx
		}
		sh := uint(7 - px)
		c := ((b0 >> sh) & 1) | (((b1 >> sh) & 1) << 1) | (((b2 >> sh) & 1) << 2) | (((b3 >> sh) & 1) << 3)

		rgb := palRGB[po+int(c)]
		v.Frame[pixOff] = byte(rgb >> 16)
		v.Frame[pixOff+1] = byte(rgb >> 8)
		v.Frame[pixOff+2] = byte(rgb)
		v.Frame[pixOff+3] = 0xFF
		bgPri[x] = prio && c != 0
	}

	// ---- Sprites ----
	v.renderSprites(line, &bgPri, blankLeft)
}

type sprite struct {
	x, tile, sy int
}

func (v *VDP) renderSprites(line int, bgPri *[ScreenW]bool, blankLeft bool) {
	if noSprites {
		return
	}
	satBase := int(v.Reg[5]&0x7E) << 7
	tall := v.Reg[1]&0x02 != 0
	zoom := v.Reg[1]&0x01 != 0
	early := v.Reg[0]&0x08 != 0
	height := 8
	if tall {
		height = 16
	}
	patBase := int(v.Reg[6]&0x04) << 11

	var buf [8]sprite
	n := 0
	overflow := false
	for i := 0; i < 64; i++ {
		y := int(v.VRAM[(satBase+i)&0x3FFF])
		if y == 0xD0 {
			break
		}
		sy0 := y + 1
		if line >= sy0 && line < sy0+height {
			if n < 8 {
				x := int(v.VRAM[(satBase+128+i*2)&0x3FFF])
				tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
				if tall {
					tile &^= 1
				}
				buf[n] = sprite{x: x, tile: tile, sy: sy0}
				n++
			} else {
				overflow = true
			}
		}
	}
	if overflow {
		v.Status |= 0x40
	}

	var used [ScreenW]bool
	var palSprite [16]uint32
	for i := 0; i < 16; i++ {
		palSprite[i] = v.color(16 + i)
	}
	lineOffset := line * ScreenW * 4

	for i := n - 1; i >= 0; i-- { // índice 0 tiene la máxima prioridad
		s := buf[i]
		syRow := line - s.sy
		if zoom {
			syRow /= 2
		}
		if syRow < 0 || syRow >= height {
			continue
		}
		pat := (patBase + s.tile*32 + syRow*4) & 0x3FFF
		b0 := v.VRAM[pat]
		b1 := v.VRAM[(pat+1)&0x3FFF]
		b2 := v.VRAM[(pat+2)&0x3FFF]
		b3 := v.VRAM[(pat+3)&0x3FFF]
		x0 := s.x
		if early {
			x0 -= 8
		}
		for px := 0; px < 8; px++ {
			sh := uint(7 - px)
			c := ((b0 >> sh) & 1) | (((b1 >> sh) & 1) << 1) | (((b2 >> sh) & 1) << 2) | (((b3 >> sh) & 1) << 3)
			if c == 0 {
				continue
			}
			rgb := palSprite[c]
			reps := 1
			if zoom {
				reps = 2
			}
			for r := 0; r < reps; r++ {
				xs := x0 + px*reps + r
				if xs < 0 || xs >= ScreenW {
					continue
				}
				if blankLeft && xs < 8 {
					continue
				}
				if bgPri[xs] {
					continue
				}
				if used[xs] {
					v.Status |= 0x20
				}
				used[xs] = true

				pixOff := lineOffset + xs*4
				v.Frame[pixOff] = byte(rgb >> 16)
				v.Frame[pixOff+1] = byte(rgb >> 8)
				v.Frame[pixOff+2] = byte(rgb)
				v.Frame[pixOff+3] = 0xFF
			}
		}
	}
}
