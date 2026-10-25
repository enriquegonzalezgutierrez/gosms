package machine

import (
	"alexkidd/pkg/cpu"
	"alexkidd/pkg/psg"
	"alexkidd/pkg/vdp"
	"os"
)

// Machine representa una Sega Master System.
type Machine struct {
	CPU           *cpu.CPU
	VDP           *vdp.VDP
	PSG           *psg.PSG
	LinesPerFrame int
}

// New crea una máquina a partir de una ROM.
func New(rom []byte) *Machine {
	c := cpu.NewCPU(rom)
	return &Machine{CPU: c, VDP: c.VDP, PSG: c.PSG, LinesPerFrame: vdp.TotalLines}
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
	lines := m.LinesPerFrame
	if lines == 0 {
		lines = vdp.TotalLines
	}
	for line := 0; line < lines; line++ {
		if line < vdp.ScreenH {
			v.RenderLine(line)
		}
		v.SetLine(line)
		m.CPU.IRQ = v.IRQ()
		m.RunCycles(vdp.CyclesPerLine)
	}
}

// SaveState guarda el estado completo de la máquina a un archivo .state
func (m *Machine) SaveState(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := m.CPU.WriteState(f); err != nil {
		return err
	}
	if err := m.VDP.WriteState(f); err != nil {
		return err
	}
	if err := m.PSG.WriteState(f); err != nil {
		return err
	}
	return nil
}

// LoadState restaura el estado completo de la máquina desde un archivo .state
func (m *Machine) LoadState(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := m.CPU.ReadState(f); err != nil {
		return err
	}
	if err := m.VDP.ReadState(f); err != nil {
		return err
	}
	if err := m.PSG.ReadState(f); err != nil {
		return err
	}
	return nil
}
