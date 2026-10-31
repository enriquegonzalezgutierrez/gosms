// Copyright (c) 2026 Enrique González Gutiérrez
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gosms/pkg/vdp"
)

type livePose struct {
	img *image.RGBA
}

// LivePoseCapture recoge las siluetas de sprites que aparecen mientras se
// juega y las guarda en una única hoja con cuadrícula. Sirve para revisar qué
// criaturas y poses faltan sin tener que volcar la ROM a mano.
type LivePoseCapture struct {
	Active    bool
	poses     map[string]livePose
	order     []string
	outDir    string
	lastWrite time.Time
}

func newLivePoseCapture(romPath string) *LivePoseCapture {
	base := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	outDir := filepath.Join("dist", "live-captura", base)
	_ = os.MkdirAll(filepath.Join(outDir, "poses"), 0o755)
	return &LivePoseCapture{
		poses:  make(map[string]livePose),
		outDir: outDir,
	}
}

func liveHash(data []byte, cram []byte) string {
	h := fnv.New64a()
	h.Write(data)
	h.Write(cram)
	return fmt.Sprintf("%016x", h.Sum64())
}

func liveABS(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func liveDecodeColor(c byte) color.RGBA {
	return color.RGBA{R: (c & 3) * 85, G: ((c >> 2) & 3) * 85, B: ((c >> 4) & 3) * 85, A: 255}
}

func liveDrawTile(dst *image.RGBA, data, cram []byte, dx, dy int) {
	for y := 0; y < 8; y++ {
		b0, b1, b2, b3 := data[y*4], data[y*4+1], data[y*4+2], data[y*4+3]
		for x := 0; x < 8; x++ {
			bit := uint(7 - x)
			c := ((b0 >> bit) & 1) | (((b1 >> bit) & 1) << 1) | (((b2 >> bit) & 1) << 2) | (((b3 >> bit) & 1) << 3)
			if c == 0 {
				continue
			}
			dst.Set(dx+x, dy+y, liveDecodeColor(cram[c&0x0F]))
		}
	}
}

type liveSprite struct {
	x, y, tile int
	key        string
	data       []byte
}

// Capture lee el SAT del frame actual y añade las poses nuevas.
func (l *LivePoseCapture) Capture(v *vdp.VDP) {
	if l == nil || !l.Active || v == nil {
		return
	}
	satBase := int(v.Reg[5]&0x7E) << 7
	tall := v.Reg[1]&0x02 != 0
	patBase := int(v.Reg[6]&0x04) << 11
	cram := v.CRAM[16:32]
	var sprites []liveSprite
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
		tile := int(v.VRAM[(satBase+128+i*2+1)&0x3FFF])
		parts := 1
		if tall {
			parts = 2
			tile &^= 1
		}
		for p := 0; p < parts; p++ {
			t := tile + p
			pat := (patBase + t*32) & 0x3FFF
			data := v.VRAM[pat : pat+32]
			sprites = append(sprites, liveSprite{x: sx, y: sy + p*8, tile: t, key: liveHash(data, cram), data: data})
		}
	}
	used := make([]bool, len(sprites))
	for i := range sprites {
		if used[i] {
			continue
		}
		used[i] = true
		q := []int{i}
		g := []liveSprite{sprites[i]}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			for j := range sprites {
				if used[j] {
					continue
				}
				if liveABS(sprites[c].x-sprites[j].x) <= 8 && liveABS(sprites[c].y-sprites[j].y) <= 8 {
					used[j] = true
					q = append(q, j)
					g = append(g, sprites[j])
				}
			}
		}
		if len(g) < 2 || len(g) > 12 {
			continue
		}
		minX, minY, maxX, maxY := 1<<30, 1<<30, -1<<30, -1<<30
		for _, s := range g {
			if s.x < minX {
				minX = s.x
			}
			if s.y < minY {
				minY = s.y
			}
			if s.x+8 > maxX {
				maxX = s.x + 8
			}
			if s.y+8 > maxY {
				maxY = s.y + 8
			}
		}
		if maxX-minX > 48 || maxY-minY > 48 {
			continue
		}
		parts := make([]string, 0, len(g))
		for _, s := range g {
			parts = append(parts, fmt.Sprintf("%s@%d,%d", s.key, s.x-minX, s.y-minY))
		}
		sort.Strings(parts)
		key := strings.Join(parts, "|")
		if _, ok := l.poses[key]; ok {
			continue
		}
		img := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
		for _, s := range g {
			liveDrawTile(img, s.data, cram, s.x-minX, s.y-minY)
		}
		l.poses[key] = livePose{img: img}
		l.order = append(l.order, key)
		if len(l.order) == 1 {
			_ = l.Write()
		}
	}
	if time.Since(l.lastWrite) >= 2*time.Second {
		_ = l.Write()
	}
}

func scaleNearest(src *image.RGBA, scale int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx()*scale, src.Bounds().Dy()*scale))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			c := src.At(x, y)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					dst.Set(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	return dst
}

// Write guarda la hoja única con todas las poses y una copia por pose.
func (l *LivePoseCapture) Write() error {
	if l == nil || len(l.order) == 0 {
		return nil
	}
	const cols = 16
	const cell = 160
	const scale = 3
	rows := (len(l.order) + cols - 1) / cols
	if rows < 4 {
		rows = 4
	}
	sheet := image.NewRGBA(image.Rect(0, 0, cols*cell, rows*cell))
	bg := color.RGBA{128, 128, 128, 255}
	border := color.RGBA{55, 55, 55, 255}
	for y := 0; y < sheet.Bounds().Dy(); y++ {
		for x := 0; x < sheet.Bounds().Dx(); x++ {
			sheet.SetRGBA(x, y, bg)
		}
	}
	for i, key := range l.order {
		p := l.poses[key]
		img := scaleNearest(p.img, scale)
		c := i % cols
		r := i / cols
		x0 := c*cell + (cell-img.Bounds().Dx())/2
		y0 := r*cell + (cell-img.Bounds().Dy())/2
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				col := img.At(x, y)
				if _, _, _, a := col.RGBA(); a > 0 {
					sheet.Set(x0+x, y0+y, col)
				}
			}
		}
		if i == 0 || i%cols == 0 || i == len(l.order)-1 {
			_ = border
		}
	}
	for r := 0; r <= rows; r++ {
		for x := 0; x < sheet.Bounds().Dx(); x++ {
			yy := r * cell
			if yy >= sheet.Bounds().Dy() {
				yy = sheet.Bounds().Dy() - 1
			}
			sheet.SetRGBA(x, yy, border)
		}
	}
	for c := 0; c <= cols; c++ {
		for y := 0; y < sheet.Bounds().Dy(); y++ {
			xx := c * cell
			if xx >= sheet.Bounds().Dx() {
				xx = sheet.Bounds().Dx() - 1
			}
			sheet.SetRGBA(xx, y, border)
		}
	}
	f, err := os.Create(filepath.Join(l.outDir, "poses.png"))
	if err != nil {
		return err
	}
	err = png.Encode(f, sheet)
	_ = f.Close()
	if err != nil {
		return err
	}
	for i, key := range l.order {
		p := l.poses[key]
		name := fmt.Sprintf("pose-%04d.png", i+1)
		ff, err := os.Create(filepath.Join(l.outDir, "poses", name))
		if err != nil {
			continue
		}
		_ = png.Encode(ff, p.img)
		_ = ff.Close()
	}
	l.lastWrite = time.Now()
	return nil
}

func (l *LivePoseCapture) Count() int {
	if l == nil {
		return 0
	}
	return len(l.order)
}
