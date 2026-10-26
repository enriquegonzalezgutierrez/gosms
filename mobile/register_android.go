//go:build android

package mobile

import "github.com/hajimehoshi/ebiten/v2/mobile"

func init() {
	mobile.SetGame(newMobileGame())
}
