package machine

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"gosms/pkg/vdp"
)

var (
	fallbackSprImg = ebiten.NewImage(8, 8)
	fallbackSprBuf = make([]byte, 8*8*4)
)

func drawFallbackSprite(screen *ebiten.Image, tileData []byte, cram []byte, drawX, drawY, scale float64) {
	pal := make([]color.RGBA, 16)
	for i := 0; i < 16; i++ {
		pal[i] = decodeColorVDP(cram[i])
	}
	hasPixels := false
	for y := 0; y < 8; y++ {
		b0 := tileData[y*4]
		b1 := tileData[y*4+1]
		b2 := tileData[y*4+2]
		b3 := tileData[y*4+3]
		for x := 0; x < 8; x++ {
			bit := 7 - x
			cIdx := ((b0 >> bit) & 1) | (((b1 >> bit) & 1) << 1) | (((b2 >> bit) & 1) << 2) | (((b3 >> bit) & 1) << 3)
			idx := (y*8 + x) * 4
			if cIdx == 0 {
				fallbackSprBuf[idx] = 0
				fallbackSprBuf[idx+1] = 0
				fallbackSprBuf[idx+2] = 0
				fallbackSprBuf[idx+3] = 0
			} else {
				c := pal[cIdx&0x0F]
				fallbackSprBuf[idx] = c.R
				fallbackSprBuf[idx+1] = c.G
				fallbackSprBuf[idx+2] = c.B
				fallbackSprBuf[idx+3] = 255
				hasPixels = true
			}
		}
	}
	if !hasPixels {
		return
	}
	fallbackSprImg.WritePixels(fallbackSprBuf)
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(drawX, drawY)
	screen.DrawImage(fallbackSprImg, op)
}

type bgDrawItem struct {
	img          *ebiten.Image
	drawX, drawY float64
	sX, sY       float64
	tX, tY       float64
}

// DrawLayer composes the remastered background and sprites for one VDP frame.
// The destination is usually an offscreen texture whose width matches
// (256-cropLeft)*scale. destX/destY let callers place the layer directly on a
// larger screen when needed.
func (hd *HDRenderer) DrawLayer(v *vdp.VDP, screen *ebiten.Image, scale, destX, destY float64, crop bool) {
	if hd == nil || !hd.Active || v == nil || screen == nil || hd.Scale < 1 {
		return
	}

	scrollX := int(v.Reg[8])
	scrollY := int(v.Reg[9])
	nameBase := int(v.Reg[2]&0x0E) << 10
	hScrollLock := v.Reg[0]&0x40 != 0

	minX := 0
	if crop {
		minX = 8
	}
	maxX := vdp.ScreenW
	cropOffset := float64(minX)
	hdScale := scale / float64(hd.Scale)

	// 1. Telón de fondo SMS
	backdropCol := decodeColorVDP(v.CRAM[16+int(v.Reg[7]&0x0F)])
	viewW := float32(float64(vdp.ScreenW-minX) * scale)
	viewH := float32(float64(vdp.ScreenH) * scale)
	vector.DrawFilledRect(screen, float32(destX), float32(destY), viewW, viewH, backdropCol, false)

	var bgLow []bgDrawItem
	var bgHigh []bgDrawItem

	// 2. Fondo Normal y Prioritario
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

			img := hd.GetHDTile(tileData, cram, false)
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

	// Capa 2: Sprites HD (con fallback continuo a píxel nativo si no está en HD)
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

			drawX := math.Floor(destX + (float64(sprX)-cropOffset)*scale)
			drawY := math.Floor(destY + float64(sprY)*scale)

			img := hd.GetHDTile(tileData, sprCram, true)
			if img != nil {
				op := &ebiten.DrawImageOptions{}
				op.Filter = ebiten.FilterLinear
				op.GeoM.Scale(hdScale, hdScale)
				op.GeoM.Translate(drawX, drawY)
				screen.DrawImage(img, op)
			} else {
				drawFallbackSprite(screen, tileData, sprCram, drawX, drawY, scale)
			}
		}
	}

	// Capa 3: Fondo Prioritario
	for _, item := range bgHigh {
		drawBgItem(item)
	}

	// 4. Máscara de recorte de bordes 4:3
	clipViewport(screen, destX, destY, float64(viewW), float64(viewH))
}

func clipViewport(screen *ebiten.Image, destX, destY, viewW, viewH float64) {
	sw, sh := float32(screen.Bounds().Dx()), float32(screen.Bounds().Dy())
	black := color.RGBA{0, 0, 0, 255}

	dX := float32(destX)
	dY := float32(destY)
	vW := float32(viewW)
	vH := float32(viewH)

	if dX > 0 {
		vector.DrawFilledRect(screen, 0, 0, dX, sh, black, false)
	}
	if dX+vW < sw {
		vector.DrawFilledRect(screen, dX+vW, 0, sw-(dX+vW), sh, black, false)
	}
	if dY > 0 {
		vector.DrawFilledRect(screen, 0, 0, sw, dY, black, false)
	}
	if dY+vH < sh {
		vector.DrawFilledRect(screen, 0, dY+vH, sw, sh-(dY+vH), black, false)
	}
}
