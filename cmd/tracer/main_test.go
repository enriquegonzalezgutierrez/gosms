package main

import (
	"bytes"
	"strings"
	"testing"

	"gosms/pkg/cpu"
)

func TestTraceInstructionsWritesRequestedSteps(t *testing.T) {
	rom := make([]byte, cpu.BankSize)
	rom[0] = 0x00 // NOP
	rom[1] = 0x00 // NOP
	c := cpu.NewCPU(rom)
	var output bytes.Buffer

	traceInstructions(&output, c, 2)

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d trace lines, want 2: %q", len(lines), output.String())
	}
	if !strings.Contains(lines[0], "[0000]") || !strings.Contains(lines[1], "[0001]") {
		t.Fatalf("trace PCs = %q, want [0000] then [0001]", output.String())
	}
}
