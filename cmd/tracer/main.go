package main

import (
	"bufio"
	"flag"
	"fmt"
	"gosms/pkg/cpu"
	"io"
	"os"
)

func traceInstructions(w io.Writer, c *cpu.CPU, steps int) {
	for i := 0; i < steps; i++ {
		line, ok := c.StepTrace()
		fmt.Fprintln(w, line)
		if !ok {
			fmt.Fprintln(w, "Ejecución detenida.")
			return
		}
	}
}

func main() {
	steps := flag.Int("n", 10000, "número máximo de instrucciones a trazar")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Uso: tracer [-n pasos] <juego.sms>")
		os.Exit(1)
	}

	rom, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error al cargar la ROM: %v\n", err)
		os.Exit(1)
	}

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	c := cpu.NewCPU(rom)

	fmt.Fprintf(w, "=== TRAZADOR Z80 - %s (%d KB, %d bancos de 16 KB) ===\n",
		flag.Arg(0), len(rom)/1024, len(rom)/cpu.BankSize)

	traceInstructions(w, c, *steps)
}
