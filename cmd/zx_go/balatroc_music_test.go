package main

// TestBalatrocMusicPlus2 — ground-truth AY capture for the pass-43 bank-5
// PT3 player. Loads music.bin at $6000 (fixed bank 5) and proof_stub.bin at
// $8000 (fixed bank 2) into a real +2 core, runs PT3 INIT then self-timed
// PLAY quarks, pumping the silent mixer once per executed frame, and records
// the exact AY mix to $BALATROC_WAV. FFT of that WAV (external) is the
// ear-parity proof. Mirrors cruxay_plus2_test.go's proven capture path.

import (
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocMusicPlus2(t *testing.T) {
	wavPath := os.Getenv("BALATROC_WAV")
	if wavPath == "" {
		wavPath = "/tmp/balatroc_music.wav"
	}
	musicPath := os.Getenv("BALATROC_MUSIC")
	if musicPath == "" {
		musicPath = "/root/nerve-workspace/demos/balatroc-c/build/music.bin"
	}
	stubPath := os.Getenv("BALATROC_STUB")
	if stubPath == "" {
		stubPath = "/root/nerve-workspace/demos/balatroc-c/music/proof_stub.bin"
	}

	music, err := os.ReadFile(musicPath)
	if err != nil {
		t.Fatalf("read music.bin: %v", err)
	}
	stub, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("read proof_stub.bin: %v", err)
	}

	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = false
	nf.headless = true
	nf.recordAudio = wavPath
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator(+2): %v", err)
	}
	emu.paused.Store(false)

	// Boot the +2 ROM to its BASIC prompt so paging/screen are settled.
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	maybeStartAudioRecording(emu.ula)

	// Poke the music room into the FIXED bank-5 window ($6000) and the
	// driver stub into fixed bank 2 ($8000). Both windows are permanent on
	// the classic +2, so plain mem writes land physically.
	for i, b := range music {
		emu.mem.Write(uint16(0x6000+i), b)
	}
	for i, b := range stub {
		emu.mem.Write(uint16(0x8000+i), b)
	}

	// Fake the RANDOMIZE USR entry the stub expects.
	emu.cpu.SP = 0x7FF0
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	// Run ~600 frames (~12 s) pumping the mixer each frame.
	for i := 0; i < 600; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		emu.ula.PumpAudioFrame()
	}
	if err := emu.ula.StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	t.Logf("captured %d bytes of AY mix -> %s ; PC=$%04X", len(music), wavPath, emu.cpu.PC)
}
