package main

// One-shot direct-AY mixer probe (delete when done): drive a standalone AY
// chip the same way the silent mixer does and confirm MixIntoStereo emits a
// real square wave, isolating chip/mix from ULA wiring and WAV writing.

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/ay"
)

func TestAYDirectMix(t *testing.T) {
	a := ay.New()
	// chA tone ~440 Hz: period 251 -> f = 1773456/16/251/... ≈ 442 Hz.
	a.WriteRegister(ay.RegMixer, 0x37) // chA tone enabled, noise off chA
	a.WriteRegister(4, 251)
	a.WriteRegister(5, 0)
	a.WriteRegister(8, 15)
	buf := make([]int16, 4096*2) // stereo interleaved
	for i := range buf {
		buf[i] = 0
	}
	a.MixIntoStereo(buf)
	min, max := int16(32767), int16(-32768)
	zc := 0
	prev := buf[0]
	for _, v := range buf {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		if (prev < 0 && v >= 0) || (prev >= 0 && v < 0) {
			zc++
		}
		prev = v
	}
	t.Logf("direct mix: min=%d max=%d zero-crossings=%d first16=%v", min, max, zc, buf[:16])
	if zc < 100 {
		t.Fatalf("no waveform — AY direct mix is DC")
	}
}

// TestAYDirectToggle counts transitions between 0 and the high rail instead of
// sign crossings (AY level output is unsigned 0..table).
func TestAYDirectToggle(t *testing.T) {
	a := ay.New()
	a.WriteRegister(ay.RegMixer, 0x37)
	a.WriteRegister(4, 251)
	a.WriteRegister(5, 0)
	a.WriteRegister(8, 15)
	buf := make([]int16, 8192*2)
	a.MixIntoStereo(buf)
	hi := 0
	tr := 0
	prev := buf[0]
	for _, v := range buf {
		if v > 1000 {
			hi++
		}
		if (prev <= 1000 && v > 1000) || (prev > 1000 && v <= 1000) {
			tr++
		}
		prev = v
	}
	// 440 Hz over 8192 samples = 8192/44100*440 ≈ 81 cycles ≈ 162 edges.
	t.Logf("high-samples=%d transitions=%d (expect ~160 edges)", hi, tr)
}
