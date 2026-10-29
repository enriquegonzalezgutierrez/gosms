// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"path/filepath"
	"testing"
)

func TestEvalScore(t *testing.T) {
	frame := make([]byte, 20*16)
	for sample := 0; sample < 20; sample++ {
		color := byte(sample % 5)
		frame[sample*16] = color * 8
		frame[sample*16+1] = color * 8
		frame[sample*16+2] = color * 8
	}
	colors, score := evalScore(frame)
	if colors != 5 {
		t.Fatalf("colors = %d, want 5", colors)
	}
	if score != 516 {
		t.Fatalf("score = %d, want 516", score)
	}
}

func TestEvalScoreUniformFrame(t *testing.T) {
	colors, score := evalScore(make([]byte, 48))
	if colors != 1 || score != 0 {
		t.Fatalf("evalScore(uniform) = (%d, %d), want (1, 0)", colors, score)
	}
}

func TestAuditRomMissingFile(t *testing.T) {
	res := auditRom(filepath.Join(t.TempDir(), "missing.sms"))
	if res.Status != "ERROR" {
		t.Fatalf("status = %q, want ERROR", res.Status)
	}
	if res.Name != "missing.sms" {
		t.Fatalf("name = %q, want missing.sms", res.Name)
	}
	if res.ErrorMsg == "" {
		t.Fatal("expected a file error")
	}
}
