// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"gosms/pkg/video"
)

func TestParsePreset(t *testing.T) {
	cases := []struct {
		name          string
		want          video.Preset
		onlyScanlines bool
	}{
		{"pixel", video.PresetPixelPerfect, false},
		{"smooth", video.PresetSmooth, false},
		{"modern", video.PresetModern, false},
		{"crt", video.PresetCRT, false},
		{"scanlines", video.PresetCRT, true},
		{"mono", video.PresetMonochrome, false},
	}
	for _, tc := range cases {
		got, scan := parsePreset(tc.name)
		if got != tc.want || scan != tc.onlyScanlines {
			t.Errorf("parsePreset(%q) = (%v, %v), want (%v, %v)", tc.name, got, scan, tc.want, tc.onlyScanlines)
		}
	}
}

func TestInputForFrameStart(t *testing.T) {
	for frame := 300; frame < 538; frame += 12 {
		pressed := inputForFrame(frame, "default")&0x10 == 0
		next := inputForFrame(frame+12, "default")&0x10 == 0
		if pressed == next {
			t.Fatalf("START should alternate around frame %d", frame)
		}
	}
}
