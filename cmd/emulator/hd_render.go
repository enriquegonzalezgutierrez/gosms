package main

import (
	"alexkidd/pkg/machine"
	"alexkidd/pkg/vdp"
	"image/color"
	"math"
	"github.com/hajimehoshi/ebiten/v2"
)

func decodeColorVDP(c byte) color.RGBA {
	r := (c & 3) * 85
	g := ((c >> 2) & 3) * 85
	b := ((c >> 4) & 3) * 85
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func (g *game) drawHDOverlay(screen *ebiten.Image, scale, destX, destY float64) {
	if machine.HD == nil || !machine.HD.Active || g.m == nil || g.m.VDP == nil {
		return
	}

	v := g.m.VDP
	scrollX := int(v.Reg[8])
	scrollY := int(v.Reg[9])
	nameBase := int(v.Reg[2]&0x0E) << 10
	hScrollLock := v.Reg[0]&0x40 != 0

	bgPalette := make([]color.RGBA, 16)
	sprPalette := make([]color.RGBA, 16)
	for i := 0; i < 16; i++ {
		bgPalette[i] = decodeColorVDP(v.CRAM[i])
		sprPalette[i] = decodeColorVDP(v.CRAM[16+i])
	}

	// 0. Capturar siempre en segundo plano los tiles visibles actuales (incluso con HD apagado)
	satBase := int(v.Reg[5]&0x7E) << 7
	patBase := int(v.Reg[6]&0x04) << 11
	tall := v.Reg[1]&0x02 != 0

	for row := 0; row < 28; row++ {
		for col := 0; col < 32; col++ {
			off := (nameBase + row*64 + col*2) & 0x3FFF
			entry := uint16(v.VRAM[(off+1)&0x3FFF])<<8 | uint16(v.VRAM[off])
			tile := int(entry & 0x1FF)
			pIdx := 0
			palToUse := bgPalette
			if entry&0x800 != 0 {
				pIdx = 1
				palToUse = sprPalette
			}
			pat := (tile * 32) & 0x3FFF
			machine.HD.RequestCapture(v.VRAM[pat:pat+32], palToUse, pIdx)
		}
	}

	for i := 0; i < 64; i++ {
		y := int(v.VRAM[(satBase+i)&0x3FFF])
		if y == 0xD0 { break }
		tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
		if tall { tile &^= 1 }
		parts := 1
		if tall { parts = 2 }
		for p := 0; p < parts; p++ {
			pat := (patBase + (tile+p)*32) & 0x3FFF
			machine.HD.RequestCapture(v.VRAM[pat:pat+32], sprPalette, 1)
		}
	}

	// Si el usuario tiene el modo HD desactivado, ya hemos capturado todo: salimos sin pintar encima
	if !g.hdMode {
		return
	}

	minX := 0
	if g.crop { minX = 8 }
	maxX := vdp.ScreenW
	cropOffset := float64(minX)
	hdScale := scale / float64(machine.HD.Scale)

	// 1. Dibujar el Fondo HD (si existe textura 4K)
	for row := 0; row < 28; row++ {
		sx := scrollX
		if hScrollLock && row < 2 { sx = 0 }

		screenY := (row*8 - scrollY) % 224
		if screenY < -8 { screenY += 224 }
		if screenY < 0 || screenY >= 192 { continue }

		for col := 0; col < 32; col++ {
			screenX := (col*8 + sx) & 0xFF

			off := (nameBase + row*64 + col*2) & 0x3FFF
			entry := uint16(v.VRAM[(off+1)&0x3FFF])<<8 | uint16(v.VRAM[off])
			tile := int(entry & 0x1FF)
			hflip := entry&0x200 != 0
			vflip := entry&0x400 != 0
			pal := entry&0x800 != 0

			pIdx := 0
			if pal { pIdx = 1 }

			pat := (tile * 32) & 0x3FFF
			tileData := v.VRAM[pat : pat+32]

			img := machine.HD.GetHDTile(tileData, pIdx)
			if img == nil {
				continue
			}

			if screenX < minX || screenX+8 > maxX { continue }

			drawX := math.Floor(destX + (float64(screenX)-cropOffset)*scale)
			drawY := math.Floor(destY + float64(screenY)*scale)

			op := &ebiten.DrawImageOptions{}
			op.Filter = ebiten.FilterLinear
			sX, sY := hdScale, hdScale
			tX, tY := 0.0, 0.0

			if hflip {
				sX = -hdScale
				tX = 8.0 * scale
			}
			if vflip {
				sY = -hdScale
				tY = 8.0 * scale
			}

			op.GeoM.Scale(sX, sY)
			op.GeoM.Translate(tX, tY)
			op.GeoM.Translate(drawX, drawY)
			screen.DrawImage(img, op)
		}
	}

	// 2. Dibujar Sprites HD
	early := v.Reg[0]&0x08 != 0

	type sprItem struct {
		x, y, tile int
	}
	var sprites []sprItem

	for i := 0; i < 64; i++ {
		y := int(v.VRAM[(satBase+i)&0x3FFF])
		if y == 0xD0 { break }
		sy := y + 1
		if sy > 240 { sy -= 256 }

		sx := int(v.VRAM[(satBase+128+i*2)&0x3FFF])
		if early { sx -= 8 }

		tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
		if tall { tile &^= 1 }

		sprites = append(sprites, sprItem{x: sx, y: sy, tile: tile})
	}

	for i := len(sprites) - 1; i >= 0; i-- {
		s := sprites[i]
		parts := 1
		if tall { parts = 2 }

		for p := 0; p < parts; p++ {
			tileIdx := s.tile + p
			pat := (patBase + tileIdx*32) & 0x3FFF
			tileData := v.VRAM[pat : pat+32]

			sprX := s.x
			sprY := s.y + p*8

			if sprX < minX-8 || sprX >= maxX || sprY < -8 || sprY >= 192 { continue }

			img := machine.HD.GetHDTile(tileData, 1)
			if img == nil {
				continue
			}

			drawX := math.Floor(destX + (float64(sprX)-cropOffset)*scale)
			drawY := math.Floor(destY + float64(sprY)*scale)

			op := &ebiten.DrawImageOptions{}
			op.Filter = ebiten.FilterLinear
			op.GeoM.Scale(hdScale, hdScale)
			op.GeoM.Translate(drawX, drawY)
			screen.DrawImage(img, op)
		}
	}
}
