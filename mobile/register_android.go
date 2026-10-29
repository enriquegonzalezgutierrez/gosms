// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build android

package mobile

import "github.com/hajimehoshi/ebiten/v2/mobile"

func init() {
	mobile.SetGame(newMobileGame())
}
