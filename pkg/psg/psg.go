// Package psg implementa el chip de sonido SN76489 de la Master System.
// Basado en emu76489 (Mitsutaka Okazaki) y MAME.
package psg

import (
	"sync"
	"sync/atomic"
)

// Clock es la frecuencia de reloj del PSG (NTSC).
const Clock = 3579545

// PSG modela el generador de sonido programable SN76489.
type PSG struct {
	mu sync.Mutex

	// Registros (escrituras por puerto $7F)
	freq      [4]uint16 // canales 0-2: período (10 bits, 0=1024). canal 3: control ruido
	vol       [4]uint8  // atenuación 0..15 (15 = silencio)
	latchCh   int
	latchType int // 0 = frecuencia, 1 = volumen

	// Estado interno (avanza a Clock/16 Hz = 223721.5625 Hz NTSC)
	toneCnt   [3]uint32
	toneOut   [3]int8 // ±1 onda cuadrada bipolar
	noiseCnt  uint32
	noiseSeed uint16 // LFSR 16 bits
	noiseMode uint8  // 0=periódico, 1=blanco
	noiseFreq uint16 // período de ruido (en ciclos PSG)
	noiseFref bool   // true si noiseFreq viene de canal 2 (freq[2])
	noiseOut  int8   // ±1 onda cuadrada que gatea ruido periódico

	sampleRate int
	// Ciclos PSG por frame de audio (fixed-point 48.16)
	// sampleRate = frames estéreo/seg = frames mono/seg (duplicados a estéreo)
	psgClkPerSample uint64
	psgClkAccum     uint64

	stateVersion uint64

	// Debug hook opcional
	WriteLog func(ch int, typ string, val int, freq uint16, vol uint8)
}

// Tabla de volúmenes lineal (emu76489/MAME), normalizada a 1.0 = vol 0.
var volTable = [16]float64{
	1.0000, 0.7969, 0.6328, 0.5000,
	0.3953, 0.3125, 0.2500, 0.2000,
	0.1563, 0.1250, 0.0977, 0.0781,
	0.0625, 0.0469, 0.0391, 0.0000,
}

// New crea un PSG con todos los canales silenciados.
func New(sampleRate int) *PSG {
	p := &PSG{sampleRate: sampleRate, noiseSeed: 0x8000}
	for i := range p.vol {
		p.vol[i] = 15
	}
	for i := range p.toneOut {
		p.toneOut[i] = 1
	}
	p.noiseOut = 1
	p.noiseFreq = 0x10
	p.noiseMode = 0
	p.noiseFref = false
	p.setRate(sampleRate)
	return p
}

// SetSampleRate ajusta la frecuencia de muestreo.
func (p *PSG) SetSampleRate(rate int) {
	p.mu.Lock()
	p.setRate(rate)
	p.mu.Unlock()
}

func (p *PSG) setRate(rate int) {
	p.sampleRate = rate
	if rate > 0 {
		p.psgClkPerSample = uint64((float64(Clock) / 16.0 / float64(rate)) * 65536.0)
	}
	p.psgClkAccum = 0
}

// Write recibe un byte escrito al puerto $7F.
func (p *PSG) Write(val byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if val&0x80 != 0 { // byte de latch
		ch := int(val>>5) & 0x03
		p.latchCh = ch
		p.latchType = int(val>>4) & 0x01
		p.setData(ch, int(val&0x0F), true)
		atomic.AddUint64(&p.stateVersion, 1)
		return
	}
	p.setData(p.latchCh, int(val&0x3F), false)
	atomic.AddUint64(&p.stateVersion, 1)
}

func (p *PSG) setData(ch, data int, latch bool) {
	if p.latchType == 1 { // volumen
		p.vol[ch] = uint8(data & 0x0F)
		if p.WriteLog != nil {
			p.WriteLog(ch, "vol", data&0x0F, p.freq[ch], p.vol[ch])
		}
		return
	}
	if ch == 3 { // ruido
		p.noiseMode = uint8((data & 0x04) >> 2)
		low := data & 0x03
		if low == 3 {
			p.noiseFreq = p.freq[2]
			p.noiseFref = true
		} else {
			p.noiseFreq = uint16(32 << low) // 0x10, 0x20, 0x40
			p.noiseFref = false
		}
		if p.noiseFreq == 0 {
			p.noiseFreq = 1
		}
		p.noiseSeed = 0x8000
		p.noiseCnt = 0
		p.noiseOut = 1
		if p.WriteLog != nil {
			p.WriteLog(ch, "noise", data&0x07, p.noiseFreq, p.vol[ch])
		}
		return
	}
	if latch {
		p.freq[ch] = (p.freq[ch] & 0x3F0) | uint16(data&0x0F)
	} else {
		p.freq[ch] = (p.freq[ch] & 0x00F) | (uint16(data&0x3F) << 4)
	}
	if p.WriteLog != nil {
		p.WriteLog(ch, "freq", 0, p.freq[ch], p.vol[ch])
	}
}

type snapshot struct {
	freq            [4]uint16
	vol             [4]uint8
	toneCnt         [3]uint32
	toneOut         [3]int8
	noiseCnt        uint32
	noiseSeed       uint16
	noiseMode       uint8
	noiseFreq       uint16
	noiseFref       bool
	noiseOut        int8
	psgClkPerSample uint64
	psgClkAccum     uint64
	sampleRate      int
	version         uint64
}

func (p *PSG) takeSnapshot() snapshot {
	p.mu.Lock()
	s := snapshot{
		freq:            p.freq,
		vol:             p.vol,
		toneCnt:         p.toneCnt,
		toneOut:         p.toneOut,
		noiseCnt:        p.noiseCnt,
		noiseSeed:       p.noiseSeed,
		noiseMode:       p.noiseMode,
		noiseFreq:       p.noiseFreq,
		noiseFref:       p.noiseFref,
		noiseOut:        p.noiseOut,
		psgClkPerSample: p.psgClkPerSample,
		psgClkAccum:     p.psgClkAccum,
		sampleRate:      p.sampleRate,
		version:         atomic.LoadUint64(&p.stateVersion),
	}
	p.mu.Unlock()
	return s
}

func (p *PSG) restoreSnapshot(s snapshot) {
	p.mu.Lock()
	p.toneCnt = s.toneCnt
	p.toneOut = s.toneOut
	p.noiseCnt = s.noiseCnt
	p.noiseSeed = s.noiseSeed
	p.noiseOut = s.noiseOut
	p.psgClkAccum = s.psgClkAccum
	p.mu.Unlock()
}

func parity9(x uint16) uint16 {
	x &= 0x0009
	x ^= x >> 4
	x ^= x >> 2
	x ^= x >> 1
	return x & 1
}

// Render mezcla audio PCM 16-bit LE MONO en out (2 bytes por frame).
// El emulador duplica a estéreo. Un frame mono = 1 frame estéreo = 1/48000 seg.
func (p *PSG) Render(out []byte) {
	s := p.takeSnapshot()
	if s.sampleRate <= 0 {
		return
	}

	// n = frames mono (2 bytes cada uno)
	n := len(out) / 2
	for i := 0; i < n; i++ {
		// Acumular ciclos PSG (fixed-point 48.16)
		oldAccumHi := uint32(s.psgClkAccum >> 16)
		s.psgClkAccum += s.psgClkPerSample
		newAccumHi := uint32(s.psgClkAccum >> 16)
		incr := newAccumHi - oldAccumHi
		s.psgClkAccum &= 0xFFFF

		if incr > 0 {
			// Canales de tono 0,1,2
			for ch := 0; ch < 3; ch++ {
				period := uint32(s.freq[ch])
				if period == 0 {
					s.toneOut[ch] = 1
					s.toneCnt[ch] = 0
					continue
				}
				s.toneCnt[ch] += incr
				for s.toneCnt[ch] >= period {
					s.toneOut[ch] = -s.toneOut[ch]
					s.toneCnt[ch] -= period
				}
			}

			// Ruido (canal 3)
			noisePeriod := uint32(s.noiseFreq)
			if s.noiseFref {
				noisePeriod = uint32(s.freq[2])
				if noisePeriod == 0 {
					noisePeriod = 1
				}
			}
			if noisePeriod == 0 {
				noisePeriod = 1
			}
			s.noiseCnt += incr
			for s.noiseCnt >= noisePeriod {
				if s.noiseMode != 0 { // blanco
					s.noiseSeed = (s.noiseSeed >> 1) | (parity9(s.noiseSeed) << 15)
				} else { // periódico
					s.noiseSeed = (s.noiseSeed >> 1) | ((s.noiseSeed & 1) << 15)
				}
				s.noiseOut = -s.noiseOut
				s.noiseCnt -= noisePeriod
			}
		}

		// Mezcla bipolar
		var mix float64

		for ch := 0; ch < 3; ch++ {
			if s.vol[ch] < 15 {
				mix += float64(s.toneOut[ch]) * volTable[s.vol[ch]]
			}
		}

		if s.vol[3] < 15 {
			var noiseSample float64
			if (s.noiseSeed & 1) != 0 {
				noiseSample = 1
			} else {
				noiseSample = -1
			}
			mix += noiseSample * volTable[s.vol[3]]
		}

		s2 := mix * 0.25
		if s2 > 1.0 {
			s2 = 1.0
		} else if s2 < -1.0 {
			s2 = -1.0
		}
		sv := int16(s2 * 32767)
		out[i*2] = byte(sv)
		out[i*2+1] = byte(sv >> 8)
	}

	p.restoreSnapshot(s)
}
