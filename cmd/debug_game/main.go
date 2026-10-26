package main

import (
	"fmt"
	"os"

	"gosms/pkg/machine"
)

func cramActive(m *machine.Machine) bool {
	for _, b := range m.VDP.CRAM {
		if b != 0 {
			return true
		}
	}
	return false
}

func main() {
	data, _ := os.ReadFile("roms/Aladdin (E) [!].sms")
	m := machine.New(data)

	// Anulamos la escritura a RAMControl (registro FFFC) interceptando cada paso
	for f := 0; f < 300; f++ {
		m.RunFrame()
		m.CPU.RAMControl = 0 // FORZAR SRAM APAGADA
	}

	if cramActive(m) {
		fmt.Printf("¡¡CRAM ACTIVA EN ALADDIN!!\n")
		return
	}
	fmt.Println("Aladdin sigue en negro")
}
