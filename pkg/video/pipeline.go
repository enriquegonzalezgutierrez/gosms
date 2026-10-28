// Package video contains the shared post-processing pipeline used by the
// desktop and mobile front ends.
package video

import (
	"embed"
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed shaders/*.kage
var shaderFS embed.FS

// Preset selects the visual treatment applied after the emulated frame has been
// composed.
type Preset int

const (
	// PresetPixelPerfect keeps hard pixel edges and does no post-processing.
	PresetPixelPerfect Preset = iota
	// PresetSmooth uses bilinear scaling without extra effects.
	PresetSmooth
	// PresetModern applies sharpening, a soft bloom and colour grading.
	PresetModern
	// PresetCRT applies a light CRT treatment on top of the modern pipeline.
	PresetCRT
	// PresetMonochrome keeps the green-phosphor look.
	PresetMonochrome
)

// Options controls the pipeline. Zero values are safe and disable the effect
// they belong to.
type Options struct {
	Preset Preset

	// HD tells the pipeline that the source is already a high-resolution
	// remastered layer. It is used only to choose the scaling filter.
	HD bool

	Bloom      float32
	Sharpness  float32
	Saturation float32
	Contrast   float32
	Gamma      float32
	Brightness float32

	Scanline  float32
	Mask      float32
	Vignette  float32
	Curvature float32
	Lines     float32
}

// DefaultOptions returns a conservative preset tuned for a 1080p display.
func DefaultOptions(preset Preset, hd bool) Options {
	opts := Options{
		Preset:     preset,
		HD:         hd,
		Saturation: 1.0,
		Contrast:   1.0,
		Gamma:      1.0,
		Brightness: 1.0,
		Lines:      192,
	}
	switch preset {
	case PresetModern:
		opts.Bloom = 0.36
		opts.Sharpness = 0.38
		opts.Saturation = 1.10
		opts.Contrast = 1.03
	case PresetCRT:
		opts.Bloom = 0.18
		opts.Sharpness = 0.20
		opts.Saturation = 1.08
		opts.Contrast = 1.04
		opts.Scanline = 0.18
		opts.Mask = 0.08
		opts.Vignette = 0.22
		opts.Curvature = 0.015
	}
	return opts
}

// Pipeline owns the intermediate render targets. It is not safe for concurrent
// use; the game loop is single-threaded.
type Pipeline struct {
	sharpen   *ebiten.Shader
	blur      *ebiten.Shader
	composite *ebiten.Shader
	crt       *ebiten.Shader

	base      *ebiten.Image
	work      *ebiten.Image
	work2     *ebiten.Image
	bloomA    *ebiten.Image
	bloomB    *ebiten.Image
	bloomBlur *ebiten.Image
	bloomUp   *ebiten.Image
	width     int
	height    int
}

// New creates a pipeline. Shader compilation is deferred until the first draw so
// the front end can report an error without crashing during start-up.
func New() *Pipeline {
	return &Pipeline{}
}

func loadShader(name string) (*ebiten.Shader, error) {
	src, err := shaderFS.ReadFile("shaders/" + name)
	if err != nil {
		return nil, err
	}
	sh, err := ebiten.NewShader(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return sh, nil
}

// compileBase loads the shaders used by the modern preset. Simple presets do
// not need any shader at all, so they keep working even on a backend that
// rejects one of the effects.
func (p *Pipeline) compileBase() error {
	if p.sharpen != nil && p.blur != nil && p.composite != nil {
		return nil
	}
	var err error
	if p.sharpen, err = loadShader("sharpen.kage"); err != nil {
		return err
	}
	if p.blur, err = loadShader("bloom_blur.kage"); err != nil {
		return err
	}
	if p.composite, err = loadShader("composite.kage"); err != nil {
		return err
	}
	return nil
}

func (p *Pipeline) compileCRT() error {
	if p.crt != nil {
		return nil
	}
	sh, err := loadShader("crt.kage")
	if err != nil {
		return err
	}
	p.crt = sh
	return nil
}

func (p *Pipeline) compileAll() error {
	if err := p.compileBase(); err != nil {
		return err
	}
	return p.compileCRT()
}

func (p *Pipeline) ensure(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if p.width == w && p.height == h && p.base != nil {
		return
	}
	p.releaseTargets()
	p.width, p.height = w, h
	p.base = ebiten.NewImage(w, h)
	p.work = ebiten.NewImage(w, h)
	p.work2 = ebiten.NewImage(w, h)
	p.bloomA = ebiten.NewImage(maxInt(1, w/2), maxInt(1, h/2))
	p.bloomB = ebiten.NewImage(maxInt(1, w/4), maxInt(1, h/4))
	p.bloomBlur = ebiten.NewImage(maxInt(1, w/4), maxInt(1, h/4))
	p.bloomUp = ebiten.NewImage(w, h)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Draw composes src (cropped to srcRect) into dst at dstRect and applies the
// selected preset. dstRect must preserve the source aspect ratio; the caller is
// responsible for centring and for clearing the letterbox bars.
func (p *Pipeline) Draw(dst, src *ebiten.Image, srcRect, dstRect image.Rectangle, opts Options) error {
	if dst == nil || src == nil || srcRect.Empty() || dstRect.Empty() {
		return nil
	}
	w, h := dstRect.Dx(), dstRect.Dy()
	p.ensure(w, h)

	p.base.Fill(color.RGBA{A: 255})
	filter := ebiten.FilterLinear
	if opts.Preset == PresetPixelPerfect && !opts.HD {
		filter = ebiten.FilterNearest
	}
	sub := src.SubImage(srcRect).(*ebiten.Image)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(w)/float64(srcRect.Dx()), float64(h)/float64(srcRect.Dy()))
	op.Filter = filter
	p.base.DrawImage(sub, op)

	drawBase := func() {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(dstRect.Min.X), float64(dstRect.Min.Y))
		op.Filter = filter
		dst.DrawImage(p.base, op)
	}

	switch opts.Preset {
	case PresetPixelPerfect, PresetSmooth:
		drawBase()
		return nil

	case PresetMonochrome:
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(dstRect.Min.X), float64(dstRect.Min.Y))
		op.Filter = ebiten.FilterNearest
		op.ColorScale.Scale(0.30, 1.20, 0.40, 1.0)
		dst.DrawImage(p.base, op)
		return nil
	}

	if err := p.compileBase(); err != nil {
		return err
	}
	if opts.Preset == PresetCRT {
		if err := p.compileCRT(); err != nil {
			return err
		}
	}

	// Sharpen pass. Even a small amount helps after scaling, but the user can
	// disable it by setting Sharpness to zero.
	source := p.base
	if opts.Sharpness > 0.001 {
		p.work.Fill(color.RGBA{A: 255})
		sop := &ebiten.DrawRectShaderOptions{}
		sop.Images[0] = p.base
		sop.Uniforms = map[string]any{"Sharpness": opts.Sharpness}
		p.work.DrawRectShader(w, h, p.sharpen, sop)
		source = p.work
	}

	// Bloom chain: half -> quarter -> blur -> upscale. The bloom is only
	// enabled for the modern and CRT presets.
	hasBloom := opts.Bloom > 0.001
	if hasBloom {
		half := &ebiten.DrawImageOptions{}
		half.Filter = ebiten.FilterLinear
		half.GeoM.Scale(float64(p.bloomA.Bounds().Dx())/float64(w), float64(p.bloomA.Bounds().Dy())/float64(h))
		p.bloomA.DrawImage(source, half)

		quarter := &ebiten.DrawImageOptions{}
		quarter.Filter = ebiten.FilterLinear
		quarter.GeoM.Scale(float64(p.bloomB.Bounds().Dx())/float64(p.bloomA.Bounds().Dx()), float64(p.bloomB.Bounds().Dy())/float64(p.bloomA.Bounds().Dy()))
		p.bloomB.DrawImage(p.bloomA, quarter)

		blur := &ebiten.DrawRectShaderOptions{}
		blur.Images[0] = p.bloomB
		blur.Uniforms = map[string]any{"Radius": float32(2.5)}
		p.bloomBlur.DrawRectShader(p.bloomB.Bounds().Dx(), p.bloomB.Bounds().Dy(), p.blur, blur)

		p.bloomUp.Fill(color.RGBA{A: 255})
		up := &ebiten.DrawImageOptions{}
		up.Filter = ebiten.FilterLinear
		up.GeoM.Scale(float64(w)/float64(p.bloomBlur.Bounds().Dx()), float64(h)/float64(p.bloomBlur.Bounds().Dy()))
		p.bloomUp.DrawImage(p.bloomBlur, up)
	}

	// Composite and grade.
	p.work2.Fill(color.RGBA{A: 255})
	cop := &ebiten.DrawRectShaderOptions{}
	cop.Images[0] = source
	if hasBloom {
		cop.Images[1] = p.bloomUp
	} else {
		// A black image avoids a second shader variant when bloom is off.
		cop.Images[1] = p.bloomUp
		p.bloomUp.Fill(color.RGBA{A: 255})
	}
	cop.Uniforms = map[string]any{
		"Bloom":      bloomValue(opts.Bloom, hasBloom),
		"Saturation": opts.Saturation,
		"Contrast":   opts.Contrast,
		"Gamma":      maxFloat(opts.Gamma, 0.01),
		"Brightness": brightness(opts.Brightness),
	}
	p.work2.DrawRectShader(w, h, p.composite, cop)

	if opts.Preset == PresetCRT {
		top := &ebiten.DrawRectShaderOptions{}
		top.Images[0] = p.work2
		top.GeoM.Translate(float64(dstRect.Min.X), float64(dstRect.Min.Y))
		top.Uniforms = map[string]any{
			"Scanline":  opts.Scanline,
			"Mask":      opts.Mask,
			"Vignette":  opts.Vignette,
			"Curvature": opts.Curvature,
			"Lines":     opts.Lines,
		}
		dst.DrawRectShader(w, h, p.crt, top)
		return nil
	}

	out := &ebiten.DrawImageOptions{}
	out.GeoM.Translate(float64(dstRect.Min.X), float64(dstRect.Min.Y))
	out.Filter = ebiten.FilterNearest
	dst.DrawImage(p.work2, out)
	return nil
}

func bloomValue(v float32, enabled bool) float32 {
	if !enabled {
		return 0
	}
	return v
}

func brightness(v float32) float32 {
	if v <= 0 {
		return 1
	}
	return v
}

func maxFloat(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func (p *Pipeline) releaseTargets() {
	for _, img := range []*ebiten.Image{p.base, p.work, p.work2, p.bloomA, p.bloomB, p.bloomBlur, p.bloomUp} {
		if img != nil {
			img.Deallocate()
		}
	}
	p.base, p.work, p.work2, p.bloomA, p.bloomB, p.bloomBlur, p.bloomUp = nil, nil, nil, nil, nil, nil, nil
}

// Dispose releases the intermediate images. Ebiten images are garbage-collected
// in normal use, but mobile front ends may want to release them explicitly when
// a ROM is unloaded.
func (p *Pipeline) Dispose() {
	p.releaseTargets()
	p.width, p.height = 0, 0
}
