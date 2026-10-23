package main

import (
	"alexkidd/pkg/machine"
	"github.com/hajimehoshi/ebiten/v2"
)

func (g *game) drawHDOverlay(screen *ebiten.Image, scale, destX, destY float64) {
	if machine.HD == nil || !machine.HD.Active {
		return
	}

	v := g.m.VDP
	scrollX := int(v.Reg[8])
	scrollY := int(v.Reg[9])
	nameBase := int(v.Reg[2]&0x0E) << 10

	hdScale := scale / float64(machine.HD.Scale)

	// 1. Dibujar el Fondo (Background)
	for row := 0; row < 28; row++ {
		for col := 0; col < 32; col++ {
			// Calcular coordenadas con scroll
			x := (col*8 - scrollX) & 0xFF
			if x > 256-8 {
				x -= 256
			}
			y := (row*8 + scrollY) % 224

			off := (nameBase + row*64 + col*2) & 0x3FFF
			entry := uint16(v.VRAM[(off+1)&0x3FFF])<<8 | uint16(v.VRAM[off])
			tile := int(entry & 0x1FF)
			pal := entry&0x800 != 0

			pIdx := 0
			if pal {
				pIdx = 1
			}

			pat := (tile * 32) & 0x3FFF
			tileData := v.VRAM[pat : pat+32]

			img := machine.HD.GetHDTile(tileData, pIdx)
			if img != nil {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(hdScale, hdScale)
				op.GeoM.Translate(destX+float64(x)*scale, destY+float64(y)*scale)
				screen.DrawImage(img, op)
			}
		}
	}

	// 2. Dibujar Sprites (OAM)
	spriteBase := int(v.Reg[5]&0x7E) << 7
	patBase := int(v.Reg[6]&4) << 11
	spriteHeight := 8
	if v.Reg[1]&2 != 0 {
		spriteHeight = 16
	}

	for i := 0; i < 64; i++ {
		sy := int(v.VRAM[spriteBase+i])
		if sy == 208 {
			break // Fin de lista de sprites
		}
		if sy > 240 {
			sy -= 256
		}
		sy++

		sx := int(v.VRAM[spriteBase+0x80+i*2])
		tile := int(v.VRAM[spriteBase+0x80+i*2+1])
		if spriteHeight == 16 {
			tile &= 0xFE
		}

		for h := 0; h < spriteHeight; h += 8 {
			pat := (patBase + tile*32 + h*4) & 0x3FFF
			tileData := v.VRAM[pat : pat+32]

			img := machine.HD.GetHDTile(tileData, 1) // Sprites usan paleta 1
			if img != nil {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(hdScale, hdScale)
				op.GeoM.Translate(destX+float64(sx)*scale, destY+float64(sy+h)*scale)
				screen.DrawImage(img, op)
			}
			tile++
		}
	}
}
