// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package mobile

import "testing"

func TestLauncherLayoutFitsCard(t *testing.T) {
	for _, size := range [][2]float64{{800, 480}, {1920, 1080}, {2400, 1080}} {
		w, h := size[0], size[1]
		empty := makeLauncher(w, h, false)
		if empty.card.w <= 0 || empty.card.h <= 0 {
			t.Fatalf("%v: empty card has no size: %+v", size, empty.card)
		}
		if !empty.choose.hit(empty.choose.x+empty.choose.w/2, empty.choose.y+empty.choose.h/2) {
			t.Fatalf("%v: choose button is not tappable", size)
		}
		if empty.choose.x < empty.card.x || empty.choose.x+empty.choose.w > empty.card.x+empty.card.w {
			t.Fatalf("%v: choose button escapes the card", size)
		}

		loaded := makeLauncher(w, h, true)
		if loaded.play.w <= 0 || loaded.change.w <= 0 {
			t.Fatalf("%v: loaded buttons have no size", size)
		}
		if loaded.play.x+loaded.play.w > loaded.change.x {
			t.Fatalf("%v: play and change overlap", size)
		}
		if loaded.play.x < loaded.card.x || loaded.change.x+loaded.change.w > loaded.card.x+loaded.card.w {
			t.Fatalf("%v: loaded buttons escape the card", size)
		}
		if loaded.help.w <= 0 || loaded.help.h <= 0 {
			t.Fatalf("%v: help button has no size", size)
		}
	}
}

func TestHUDDirectionsCoverFourBits(t *testing.T) {
	hud := makeHUD(1920, 1080)
	seen := byte(0)
	for _, dir := range hud.directions() {
		if dir.bit == 0 {
			t.Fatalf("direction %q has no pad bit", dir.label)
		}
		seen |= dir.bit
		if dir.r <= 0 {
			t.Fatalf("direction %q has no radius", dir.label)
		}
	}
	if seen != 0x0F {
		t.Fatalf("direction bits = %04b, want 1111", seen)
	}
	if hud.b1X <= hud.b2X || hud.btnR <= 0 {
		t.Fatalf("right buttons are not laid out: %+v", hud)
	}
	if hud.menu.hit(hud.pause.x, hud.pause.y) {
		t.Fatalf("menu and pause overlap")
	}
}

func TestROMNameAndSize(t *testing.T) {
	if got := cleanROMName("Alex_Kidd (UE) [!].sms"); got != "Alex Kidd (UE) [!]" {
		t.Fatalf("cleanROMName = %q", got)
	}
	if got := shortName("abcdefghij", 6); got != "abc..." {
		t.Fatalf("shortName = %q", got)
	}
	if got := formatKB(512 * 1024); got != "512 KB" {
		t.Fatalf("formatKB = %q", got)
	}
}
