package main

import (
	"alexkidd/pkg/machine"
	"alexkidd/pkg/vdp"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

func decodeColorVDP(c byte) color.RGBA {
	r := (c & 3) * 85
	g := ((c >> 2) & 3) * 85
	b := ((c >> 4) & 3) * 85
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

type bgDrawItem struct {
	img          *ebiten.Image
	drawX, drawY float64
	sX, sY       float64
	tX, tY       float64
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

	minX := 0
	if g.crop {
		minX = 8
	}
	maxX := vdp.ScreenW
	cropOffset := float64(minX)
	hdScale := scale / float64(machine.HD.Scale)

	// 1. Fondo de Telón Base (Backdrop SMS)
	backdropCol := decodeColorVDP(v.CRAM[16+int(v.Reg[7]&0x0F)])
	viewW := float32(float64(vdp.ScreenW-minX) * scale)
	viewH := float32(float64(vdp.ScreenH) * scale)
	vector.DrawFilledRect(screen, float32(destX), float32(destY), viewW, viewH, backdropCol, false)

	var bgLow []bgDrawItem
	var bgHigh []bgDrawItem

	// 2. Clasificar Tiles de Fondo en Normal y Prioritario
	fineY := scrollY & 7
	for r := -1; r < 25; r++ {
		tileDrawY := r*8 - fineY
		if tileDrawY+8 <= 0 || tileDrawY >= 192 {
			continue
		}

		bgY := tileDrawY + scrollY
		row := (bgY >> 3) % 28

		sx := scrollX
		if hScrollLock && (tileDrawY < 16) {
			sx = 0
		}
		fineX := sx & 7

		for c := -1; c < 33; c++ {
			tileDrawX := c*8 + fineX
			if tileDrawX+8 <= minX || tileDrawX >= maxX {
				continue
			}

			bgX := tileDrawX - sx
			col := (bgX >> 3) & 0x1F

			off := (nameBase + row*64 + col*2) & 0x3FFF
			entry := uint16(v.VRAM[(off+1)&0x3FFF])<<8 | uint16(v.VRAM[off])
			tile := int(entry & 0x1FF)
			hflip := entry&0x200 != 0
			vflip := entry&0x400 != 0
			pal := entry&0x800 != 0
			prio := entry&0x1000 != 0

			palOffset := 0
			if pal {
				palOffset = 16
			}
			pat := (tile * 32) & 0x3FFF
			tileData := v.VRAM[pat : pat+32]
			cram := v.CRAM[palOffset : palOffset+16]

			img := machine.HD.GetHDTile(tileData, cram, false)
			if img != nil {
				drawX := math.Floor(destX + (float64(tileDrawX)-cropOffset)*scale)
				drawY := math.Floor(destY + float64(tileDrawY)*scale)

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

				item := bgDrawItem{
					img:   img,
					drawX: drawX, drawY: drawY,
					sX: sX, sY: sY,
					tX: tX, tY: tY,
				}
				if prio {
					bgHigh = append(bgHigh, item)
				} else {
					bgLow = append(bgLow, item)
				}
			}
		}
	}

	drawBgItem := func(item bgDrawItem) {
		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterLinear
		op.GeoM.Scale(item.sX, item.sY)
		op.GeoM.Translate(item.tX, item.tY)
		op.GeoM.Translate(item.drawX, item.drawY)
		screen.DrawImage(item.img, op)
	}

	// Capa 1: Fondo Normal
	for _, item := range bgLow {
		drawBgItem(item)
	}

	// Capa 2: Sprites HD
	satBase := int(v.Reg[5]&0x7E) << 7
	patBase := int(v.Reg[6]&0x04) << 11
	tall := v.Reg[1]&0x02 != 0
	early := v.Reg[0]&0x08 != 0
	sprCram := v.CRAM[16:32]

	type sprItem struct{ x, y, tile int }
	var sprites []sprItem

	for i := 0; i < 64; i++ {
		y := int(v.VRAM[(satBase+i)&0x3FFF])
		if y == 0xD0 {
			break
		}
		sy := y + 1
		if sy > 240 {
			sy -= 256
		}
		sx := int(v.VRAM[(satBase+128+i*2)&0x3FFF])
		if early {
			sx -= 8
		}
		tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
		if tall {
			tile &^= 1
		}
		sprites = append(sprites, sprItem{x: sx, y: sy, tile: tile})
	}

	for i := len(sprites) - 1; i >= 0; i-- {
		s := sprites[i]
		parts := 1
		if tall {
			parts = 2
		}

		for p := 0; p < parts; p++ {
			tileIdx := s.tile + p
			pat := (patBase + tileIdx*32) & 0x3FFF
			tileData := v.VRAM[pat : pat+32]
			sprX := s.x
			sprY := s.y + p*8

			if sprX < minX-8 || sprX >= maxX || sprY < -8 || sprY >= 192 {
				continue
			}

			img := machine.HD.GetHDTile(tileData, sprCram, true)
			if img != nil {
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

	// Capa 3: Fondo Prioritario (hojas de palmeras, HUD, etc.)
	for _, item := range bgHigh {
		drawBgItem(item)
	}

	// 4. Recorte perfecto de bordes (Overscan Pillarbox)
	clipViewportHD(screen, destX, destY, float64(viewW), float64(viewH))
}

// ClipViewportHD enmascara los bordes exteriores garantizando un corte limpio 4:3
func clipViewportHD(screen *ebiten.Image, destX, destY, viewW, viewH float64) {
	sw, sh := float32(screen.Bounds().Dx()), float32(screen.Bounds().Dy())
	black := color.RGBA{0, 0, 0, 255}

	dX := float32(destX)
	dY := float32(destY)
	vW := float32(viewW)
	vH := float32(viewH)

	// Barra izquierda
	if dX > 0 {
		vector.DrawFilledRect(screen, 0, 0, dX, sh, black, false)
	}
	// Barra derecha
	if dX+vW < sw {
		vector.DrawFilledRect(screen, dX+vW, 0, sw-(dX+vW), sh, black, false)
	}
	// Barra superior
	if dY > 0 {
		vector.DrawFilledRect(screen, 0, 0, sw, dY, black, false)
	}
	// Barra inferior
	if dY+vH < sh {
		vector.DrawFilledRect(screen, 0, dY+vH, sw, sh-(dY+vH), black, false)
	}
}
