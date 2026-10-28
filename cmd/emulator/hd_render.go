package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"gosms/pkg/machine"
)

func decodeColorVDP(c byte) color.RGBA {
	r := (c & 3) * 85
	g := ((c >> 2) & 3) * 85
	b := ((c >> 4) & 3) * 85
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// drawHDLayer is a thin adapter around machine.HDRenderer.DrawLayer. The
// renderer lives in pkg/machine so the preview tool and the Android front end
// can reuse it.
func (g *game) drawHDLayer(screen *ebiten.Image, scale, destX, destY float64) {
	if g.m == nil || g.m.VDP == nil || machine.HD == nil {
		return
	}
	machine.HD.DrawLayer(g.m.VDP, screen, scale, destX, destY, g.crop)
}
