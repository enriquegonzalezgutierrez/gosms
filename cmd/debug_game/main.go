package main

import (
	"fmt"
	"os"

	"alexkidd/pkg/machine"
)

func main() {
	dataL, _ := os.ReadFile("roms/Loletta no Syouzou (J) [!].sms")
	if len(dataL)%1024 == 512 { dataL = dataL[512:] }
	m := machine.New(dataL)

	for i := 0; i < 200000; i++ {
		pc := m.CPU.PC
		iff1 := m.CPU.IFF1
		name, _, ok := m.CPU.Step()
		if !ok { break }
		if name == "EI" {
			fmt.Printf("¡EI ejecutado en paso %d, PC=0x%04X!\n", i, pc)
		}
		if pc == 0x0112 {
			fmt.Printf("¡Llegó al bucle de espera en paso %d! IFF1=%t, SP=0x%04X\n", i, iff1, m.CPU.SP)
			break
		}
	}
}
