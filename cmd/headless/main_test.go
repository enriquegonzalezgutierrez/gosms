// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wav")
	pcm := []byte{0x34, 0x12, 0xFE, 0xFF}
	if err := writeWAV(path, pcm, 48000); err != nil {
		t.Fatalf("writeWAV() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 44+len(pcm) {
		t.Fatalf("file length = %d, want %d", len(data), 44+len(pcm))
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || string(data[36:40]) != "data" {
		t.Fatalf("invalid WAV markers: %.4q %.4q %.4q", data[:4], data[8:12], data[36:40])
	}
	if got := binary.LittleEndian.Uint32(data[24:28]); got != 48000 {
		t.Fatalf("sample rate = %d, want 48000", got)
	}
	if got := binary.LittleEndian.Uint32(data[40:44]); got != uint32(len(pcm)) {
		t.Fatalf("data size = %d, want %d", got, len(pcm))
	}
	if string(data[44:]) != string(pcm) {
		t.Fatalf("PCM payload = %v, want %v", data[44:], pcm)
	}
}
