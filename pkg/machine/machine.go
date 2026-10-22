package machine

import (
	"alexkidd/pkg/cpu"
	"alexkidd/pkg/psg"
	"alexkidd/pkg/vdp"
)

// Machine representa una Sega Master System.
type Machine struct {
	CPU *cpu.CPU
	VDP *vdp.VDP
	PSG *psg.PSG
}

// New crea una máquina a partir de una ROM.
func New(rom []byte) *Machine {
	c := cpu.NewCPU(rom)
	return &Machine{CPU: c, VDP: c.VDP, PSG: c.PSG}
}

// RunCycles ejecuta la CPU hasta consumir al menos n ciclos.
func (m *Machine) RunCycles(n int) {
	target := m.CPU.Cycles + uint64(n)
	for m.CPU.Cycles < target {
		_, _, ok := m.CPU.Step()
		if !ok {
			break
		}
	}
}

// RunFrame ejecuta un frame completo (262 líneas) y renderiza el vídeo.
func (m *Machine) RunFrame() {
	v := m.VDP
	for line := 0; line < vdp.TotalLines; line++ {
		if line < vdp.ScreenH {
			v.RenderLine(line)
		}
		v.SetLine(line)
		m.CPU.IRQ = v.IRQ()
		m.RunCycles(vdp.CyclesPerLine)
	}
}
