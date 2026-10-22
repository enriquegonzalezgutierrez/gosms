package psg

import (
	"math"
	"testing"
)

// countToggles cuenta los cambios de signo para estimar la frecuencia.
func countToggles(pcm []byte) int {
	n := len(pcm) / 2
	toggles := 0
	prev := int16(0)
	for i := 0; i < n; i++ {
		s := int16(uint16(pcm[i*2]) | uint16(pcm[i*2+1])<<8)
		if i > 0 && ((s < 0) != (prev < 0)) {
			toggles++
		}
		prev = s
	}
	return toggles
}

// TestToneFrequency comprueba el canal y la afinación: un tono de frecuencia N
// debe sonar a Clock/(32*N) con independencia del canal elegido.
func TestToneFrequency(t *testing.T) {
	const rate = 48000
	for _, tc := range []struct {
		ch   int
		n    int
		want float64
	}{
		{0, 254, float64(Clock) / (32 * 254)},
		{1, 400, float64(Clock) / (32 * 400)},
		{2, 100, float64(Clock) / (32 * 100)},
	} {
		p := New(rate)
		latch := byte(0x80 | tc.ch<<5) // frecuencia del canal tc.ch
		p.Write(latch | byte(tc.n&0x0F))
		p.Write(byte(tc.n >> 4))       // nibble alto
		p.Write(byte(0x90 | tc.ch<<5)) // volumen máximo en el mismo canal

		pcm := make([]byte, rate*2)
		p.Render(pcm)

		toggles := countToggles(pcm)
		got := float64(toggles) / 2 // una conmutación por semiciclo
		if math.Abs(got-tc.want) > tc.want*0.02 {
			t.Errorf("canal %d N=%d: frecuencia %.1f Hz, esperada %.1f Hz",
				tc.ch, tc.n, got, tc.want)
		}
	}
}

// TestVolumeSilence comprueba que la atenuación máxima silencia el canal.
func TestVolumeSilence(t *testing.T) {
	p := New(48000)
	p.Write(0x80 | 0x00) // latch frecuencia ch0, nibble bajo 0
	p.Write(0x02)        // nibble alto -> N=32
	p.Write(0x90 | 0x0F) // volumen ch0 = 15 (máximo silencio)
	pcm := make([]byte, 48000*2)
	p.Render(pcm)
	for i := 0; i < len(pcm); i++ {
		if pcm[i] != 0 {
			t.Fatalf("se esperaba silencio, byte %d = %d", i, pcm[i])
		}
	}
}

// TestChannelSelection comprueba que un latch de volumen en el canal 1 no
// afecta a los demás canales.
func TestChannelSelection(t *testing.T) {
	p := New(48000)
	// canal 0: tono audible
	p.Write(0x80 | 0x00)
	p.Write(0x02)
	p.Write(0x90) // volumen ch0 = 0 (máximo)
	// canal 1: volumen a silencio
	p.Write(0xA0 | 0x0F)

	pcm := make([]byte, 48000*2)
	p.Render(pcm)

	// Debe sonar (canal 0), pero con el volumen repartido no debe llegar al
	// máximo; basta comprobar que hay señal.
	var peak int
	for i := 0; i+1 < len(pcm); i += 2 {
		s := int(int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8))
		if s < 0 {
			s = -s
		}
		if s > peak {
			peak = s
		}
	}
	if peak == 0 {
		t.Fatal("el canal 0 debería sonar")
	}
}

// TestConcurrentRenderAndWrite reproduce el uso real: el hilo de audio pide
// muestras mientras la emulación escribe registros. Con -race detecta carreras.
func TestConcurrentRenderAndWrite(t *testing.T) {
	p := New(48000)
	p.Write(0x90) // volumen máximo en el canal 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			buf := make([]byte, 1024)
			p.Render(buf)
		}
	}()
	for i := 0; i < 2000; i++ {
		p.Write(byte(0x80 | (i & 0x0F)))
		p.Write(byte((i >> 4) & 0x3F))
	}
	<-done
}
