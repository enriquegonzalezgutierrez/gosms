// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"gosms/pkg/machine"
)

func TestCramActive(t *testing.T) {
	m := machine.New(make([]byte, 0x4000))
	if cramActive(m) {
		t.Fatal("new machine should have an inactive CRAM")
	}
	m.VDP.CRAM[3] = 1
	if !cramActive(m) {
		t.Fatal("nonzero CRAM entry should be active")
	}
}
