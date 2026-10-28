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

	Bloom          float32
	BloomThreshold float32
	Exposure       float32
	ToneMap        float32
	Dither         float32
	Sharpness      float32
	Saturation     float32
	Contrast       float32
	Gamma          float32
	Brightness     float32

	// Height es el mapa de altura compuesto (misma escena que el color). Si
	// viene, la iluminacion usa volumen real en vez del gradiente de color.
	Height *ebiten.Image

	// Cielo moderno: sustituye el color plano del cielo por un degradado con
	// halo de sol. SkyColor lo pone el front end a partir del fondo del VDP.
	Sky        bool
	SkyColor   [3]float32
	SkyTop     [3]float32
	SkyHorizon [3]float32
	SkySun     [2]float32
	SkyGlow    float32

	// Iluminacion de relieve (solo tiene sentido con arte HD).
	Relief          float32
	ReliefStep      float32
	ReliefLightX    float32
	ReliefLightY    float32
	ReliefAmbient   float32
	ReliefDiffuse   float32
	ReliefSpecular  float32
	ReliefGloss     float32
	ReliefRim       float32
	HeightLit       float32
	HeightSlope     float32
	HeightOcclusion float32

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
		SkyTop:     [3]float32{0.10, 0.24, 0.42},
		SkyHorizon: [3]float32{0.38, 0.58, 0.72},
		SkySun:     [2]float32{0.24, 0.18},
		SkyGlow:    0.16,
		Saturation: 1.0,
		Contrast:   1.0,
		Gamma:      1.0,
		Brightness: 1.0,
		Exposure:   1.0,
		Dither:     1.0,
		Lines:      192,
	}
	switch preset {
	case PresetModern:
		opts.Bloom = 0.30
		opts.BloomThreshold = 0.68
		opts.Exposure = 1.00
		opts.ToneMap = 1
		opts.Sharpness = 0.38
		opts.Saturation = 1.02
		opts.Contrast = 1.02
		opts.Relief = 1.5
		opts.ReliefStep = 2.0
		opts.ReliefLightX = -0.38
		opts.ReliefLightY = -0.72
		opts.ReliefAmbient = 0.72
		opts.ReliefDiffuse = 0.66
		opts.ReliefSpecular = 0.07
		opts.ReliefGloss = 18.0
		opts.ReliefRim = 0.04
		opts.HeightLit = 1
		opts.HeightSlope = 4.2
		opts.HeightOcclusion = 0.6
		// Con mapa de altura la referencia es la superficie plana: ambiental
		// 1.0 mantiene el cielo y las zonas lisas con su color original.
		opts.ReliefAmbient = 1.0
		opts.ReliefDiffuse = 0.90
		if hd {
			// El arte vectorizado ya llega limpio. El sharpen sobre el escalado
			// hacia vibrar el decorado al desplazarse y anadia otro pase
			// full-screen; en HD no aporta y se desactiva.
			opts.Sharpness = 0
			opts.ReliefSpecular = 0.04
			opts.ReliefRim = 0.02
		}
	case PresetCRT:
		opts.Bloom = 0.18
		opts.BloomThreshold = 0.55
		opts.Exposure = 1.02
		opts.ToneMap = 1
		opts.Sharpness = 0.20
		opts.Saturation = 1.08
		opts.Contrast = 1.04
		opts.Scanline = 0.18
		opts.Mask = 0.08
		opts.Vignette = 0.22
		opts.Curvature = 0.015
		opts.ReliefLightX = -0.38
		opts.ReliefLightY = -0.72
		opts.ReliefAmbient = 1.0
		opts.ReliefDiffuse = 0.70
		opts.ReliefSpecular = 0.05
		opts.ReliefGloss = 14.0
		opts.ReliefRim = 0.03
		opts.HeightLit = 1
		opts.HeightSlope = 4.5
		opts.HeightOcclusion = 0.6
		if hd {
			opts.Sharpness = 0
			opts.ReliefSpecular = 0.03
			opts.ReliefRim = 0.015
		}
	}
	// El remaster usa albedo plano: sin pase de altura ni relieve dinamico.
	opts.HeightLit = 0
	opts.Relief = 0
	return opts
}

// Pipeline owns the intermediate render targets. It is not safe for concurrent
// use; the game loop is single-threaded.
type Pipeline struct {
	sharpen   *ebiten.Shader
	relief    *ebiten.Shader
	heightLit *ebiten.Shader
	blur      *ebiten.Shader
	composite *ebiten.Shader
	crt       *ebiten.Shader

	base      *ebiten.Image
	light     *ebiten.Image
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
	if p.sharpen != nil && p.blur != nil && p.composite != nil && p.relief != nil && p.heightLit != nil {
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
	if p.relief, err = loadShader("relief.kage"); err != nil {
		return err
	}
	if p.heightLit, err = loadShader("heightlight.kage"); err != nil {
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
	p.light = ebiten.NewImage(w, h)
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

	// Relief lighting. Only meaningful on remastered art: at native resolution
	// the gradient would be a single game pixel wide and would just emboss noise.
	source := p.base
	if opts.Height != nil && opts.HeightLit > 0.001 {
		// El mapa de altura vive a la resolucion del pack, que no tiene por que
		// coincidir con la ventana. Se escala una sola vez a la resolucion de
		// destino para que el gradiente se mida en pixeles reales de pantalla.
		heightSrc := opts.Height
		if heightSrc.Bounds().Dx() != w || heightSrc.Bounds().Dy() != h {
			p.work.Fill(color.RGBA{A: 255})
			scaleOp := &ebiten.DrawImageOptions{}
			scaleOp.Filter = ebiten.FilterLinear
			scaleOp.GeoM.Scale(float64(w)/float64(heightSrc.Bounds().Dx()), float64(h)/float64(heightSrc.Bounds().Dy()))
			p.work.DrawImage(heightSrc, scaleOp)
			heightSrc = p.work
		}
		p.light.Fill(color.RGBA{A: 255})
		hop := &ebiten.DrawRectShaderOptions{}
		hop.Images[0] = p.base
		hop.Images[1] = heightSrc
		hop.Uniforms = map[string]any{
			"Light":     []float32{opts.ReliefLightX, opts.ReliefLightY},
			"Ambient":   reliefValue(opts.ReliefAmbient, 1),
			"Diffuse":   opts.ReliefDiffuse,
			"Specular":  opts.ReliefSpecular,
			"Gloss":     reliefValue(opts.ReliefGloss, 1),
			"Rim":       opts.ReliefRim,
			"Slope":     opts.HeightSlope,
			"Occlusion": opts.HeightOcclusion,
		}
		p.light.DrawRectShader(w, h, p.heightLit, hop)
		source = p.light
	} else if opts.Relief > 0.001 && opts.HD {
		p.light.Fill(color.RGBA{A: 255})
		lop := &ebiten.DrawRectShaderOptions{}
		lop.Images[0] = p.base
		lop.Uniforms = map[string]any{
			"Step":     []float32{reliefStep(opts.ReliefStep), reliefStep(opts.ReliefStep)},
			"Light":    []float32{opts.ReliefLightX, opts.ReliefLightY},
			"Ambient":  reliefValue(opts.ReliefAmbient, 1),
			"Diffuse":  opts.ReliefDiffuse,
			"Specular": opts.ReliefSpecular,
			"Gloss":    reliefValue(opts.ReliefGloss, 1),
			"Rim":      opts.ReliefRim,
			"Relief":   opts.Relief,
		}
		p.light.DrawRectShader(w, h, p.relief, lop)
		source = p.light
	}

	// Sharpen pass. Even a small amount helps after scaling, but the user can
	// disable it by setting Sharpness to zero.
	if opts.Sharpness > 0.001 {
		p.work.Fill(color.RGBA{A: 255})
		sop := &ebiten.DrawRectShaderOptions{}
		sop.Images[0] = source
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

		up := &ebiten.DrawImageOptions{}
		up.Filter = ebiten.FilterLinear
		up.GeoM.Scale(float64(w)/float64(p.bloomBlur.Bounds().Dx()), float64(h)/float64(p.bloomBlur.Bounds().Dy()))
		p.bloomUp.DrawImage(p.bloomBlur, up)
	}

	// Composite and grade.
	cop := &ebiten.DrawRectShaderOptions{}
	cop.Images[0] = source
	if hasBloom {
		cop.Images[1] = p.bloomUp
	} else {
		// A black image avoids a second shader variant when bloom is off.
		cop.Images[1] = p.bloomUp
	}
	cop.Uniforms = map[string]any{
		"Bloom":          bloomValue(opts.Bloom, hasBloom),
		"BloomThreshold": opts.BloomThreshold,
		"Saturation":     opts.Saturation,
		"Contrast":       opts.Contrast,
		"Gamma":          maxFloat(opts.Gamma, 0.01),
		"Brightness":     brightness(opts.Brightness),
		"Exposure":       exposureValue(opts.Exposure),
		"ToneMap":        opts.ToneMap,
		"Dither":         opts.Dither,
		"Sky":            skyValue(opts.Sky),
		"SkyColor":       []float32{opts.SkyColor[0], opts.SkyColor[1], opts.SkyColor[2]},
		"TopColor":       []float32{opts.SkyTop[0], opts.SkyTop[1], opts.SkyTop[2]},
		"HorizonColor":   []float32{opts.SkyHorizon[0], opts.SkyHorizon[1], opts.SkyHorizon[2]},
		"Sun":            []float32{opts.SkySun[0], opts.SkySun[1]},
		"Glow":           opts.SkyGlow,
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

func skyValue(enabled bool) float32 {
	if enabled {
		return 1
	}
	return 0
}

func bloomValue(v float32, enabled bool) float32 {
	if !enabled {
		return 0
	}
	return v
}

func reliefStep(v float32) float32 {
	if v < 0.1 {
		return 1
	}
	return v
}

func reliefValue(v, fallback float32) float32 {
	if v <= 0 {
		return fallback
	}
	return v
}

func exposureValue(v float32) float32 {
	if v <= 0 {
		return 1
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
	for _, img := range []*ebiten.Image{p.base, p.light, p.work, p.work2, p.bloomA, p.bloomB, p.bloomBlur, p.bloomUp} {
		if img != nil {
			img.Deallocate()
		}
	}
	p.base, p.light, p.work, p.work2, p.bloomA, p.bloomB, p.bloomBlur, p.bloomUp = nil, nil, nil, nil, nil, nil, nil, nil
}

// Dispose releases the intermediate images. Ebiten images are garbage-collected
// in normal use, but mobile front ends may want to release them explicitly when
// a ROM is unloaded.
func (p *Pipeline) Dispose() {
	p.releaseTargets()
	p.width, p.height = 0, 0
}
