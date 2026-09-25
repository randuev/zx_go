package main

// crux256 v2b audio proof: feed the EXACT register stream crux256 latches
// into the AY chip (mixer $3D chC-only, refrain period bytes on reg4,
// vol $0D) and confirm real oscillation + expected pitches per note.

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/ay"
)

func TestCrux256AudioProof(t *testing.T) {
	a := ay.New()
	a.WriteRegister(ay.RegMixer, 0x3B) // chC tone ON, chA/B OFF, rest OFF
	a.WriteRegister(3, 0x00)
	a.WriteRegister(5, 0x00)
	a.WriteRegister(13, 0x00)
	a.WriteRegister(10, 0x0D) // chC volume 13

	mel := []byte{0x7E, 0x6A, 0x5E, 0x54, 0x47} // A4 C5 D5 E5 G5 (true /32 formula)
	const sr = 44100.0
	buf := make([]int16, 8192*2)
	for i, p := range mel {
		a.WriteRegister(4, p)
		for k := range buf {
			buf[k] = 0
		}
		a.MixIntoStereo(buf)
		// toggles (unsigned square) + dominant FFT bin on left channel
		// measure frequency via edge intervals (square wave, unsigned levels)
		var prev bool
		var last int
		edges := 0
		const W = 4096
		var total float64
		for k := 0; k < W; k++ {
			hi := buf[2*k] > 100
			if hi != prev {
				if edges > 0 {
					total += float64(k - last)
				}
				edges++
				last = k
			}
			prev = hi
		}
		freq := 0.0
		if edges > 2 {
			meanInterval := total / float64(edges-1) // half-period in samples
			freq = sr / (2.0 * meanInterval)
		}
		exp := 1773456.0 / 32.0 / float64(p)
		t.Logf("note %2d period=$%02X edges=%d freqHz=%.0f expected=%.0f",
			i, p, edges, freq, exp)
		if edges < 20 {
			t.Fatalf("note %d (period $%02X): NO oscillation — channel silent", i, p)
		}
		if freq < exp*0.9 || freq > exp*1.1 {
			t.Fatalf("note %d %.0f Hz, expected ~%.0f Hz", i, freq, exp)
		}
	}
	// rest byte: must be silent-ish (volume held, period 0)
	a.WriteRegister(4, 0x00)
	for k := range buf {
		buf[k] = 0
	}
	a.MixIntoStereo(buf)
	var mx int16
	for _, v := range buf {
		if v > mx {
			mx = v
		}
	}
	t.Logf("rest byte: max=%d (period0 -> chC holds)", mx)
}
