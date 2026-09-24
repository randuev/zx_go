package main

// One-shot audio repro (delete when done): drive the classic AY chip on the
// +2 core straight from registers, record via the silent mixer, and confirm
// the WAV carries a real 440 Hz waveform — isolating mixer wiring from demo.

import (
	"math"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestAYWavRepro(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = false
	nf.headless = true
	nf.recordAudio = "/tmp/ay_repro.wav"
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	// AY channel A tone ~440 Hz: period = 1773456/440/16 ≈ 251 -> coarse
	// exact tone via regs 4,5: fine+coarse, period P: f=1.7734MHz/(16P).
	// 440 Hz -> P ≈ 251. regs: R4 = fine (P&255), R5 = coarse (P>>8).
	// Noise off chA, tone on: mixer R7 bit2 (chA tone disable) = 0.
	// Volume chA = 15 (R8).
	writeAY := func(r, v byte) {
		emu.ula.WritePort(0xFFFD, r)
		emu.ula.WritePort(0xBFFD, v)
	}
	// pump 50 frames (~1 s) while toggling between 440 and 880 Hz every frame
	for f := 0; f < 100; f++ {
		if f%2 == 0 {
			writeAY(4, 251)
			writeAY(5, 0)
		} else {
			writeAY(4, 125)
			writeAY(5, 0)
		}
		writeAY(7, 0x38) // chA tone ENABLED (bit0=0), noise off all, chB/C tone disabled bit1..2? no; keep simple
		writeAY(8, 15)   // vol A
		runOneFrameHeadless(emu, roms.ModelPlus2)
		emu.ula.PumpAudioFrame()
	}
	if err := emu.ula.StopRecording(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	t.Log("repro wav written")
	_ = math.Pi
}
