package main

// TestBalatrocMusicPlus2 — pass-48b ISOLATED SOUND CAPTURE (Seva's law:
// "play these sounds in isolation, record them, iterate until they're right").
// The pass-43 PT3/$6000-bridge test it replaces is OBSOLETE — bulba retired,
// the AY player lives inside the board block now (music_isr.asm) and every
// note is a table in music.c.
//
// This test hijacks the REAL game at the CRT entry, runs the title screen,
// and records ONE WAV of pure title music. SFX are recorded by the sibling
// recorder (balatroc_record_test.go) over the full round; this file's job is
// the MUSIC ITSELF, clean, no input noise: 8 seconds of the title song,
// nothing else. Gate: MUSIC=1; output $BALATROC_WAV (default /tmp/balatroc_music.wav).
//
// PASS: WAV exists, has real AY content — verified by RMS > silence floor on
// the DC-removed signal (flat-rail captures used to fake success until the
// EnableAudioSilent attach law was enforced).

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocMusicPlus2(t *testing.T) {
	if os.Getenv("MUSIC") == "" {
		t.Skip("set MUSIC=1")
	}
	wavPath := os.Getenv("BALATROC_WAV")
	if wavPath == "" {
		wavPath = "/tmp/balatroc_music.wav"
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
	}
	if bd == nil {
		t.Fatal("board CODE block missing from tap")
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.headless = true
	nf.recordAudio = wavPath // pump gating: PumpAudioFrame only feeds samples while the record flag is wired
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	if emu.ula == nil {
		t.Fatal("no ula")
	}
	emu.ula.EnableAudioSilent() // THE attach law — StartRecording is a no-op without it
	emu.paused.Store(true)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFFE
	emu.mem.Write(0xFFFC, byte(sent))
	emu.mem.Write(0xFFFD, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.Halted = false
	emu.cpu.D, emu.cpu.E, emu.cpu.H, emu.cpu.L = 0, 0, 0x80, 0x00
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
			emu.ula.PumpAudioFrame()
		}
	}
	runF(60) // let the CRT boot + title music start
	if err := emu.ula.StartRecording(wavPath); err != nil {
		t.Fatalf("rec start: %v", err)
	}
	runF(400) // 8.0 s of title music, NO keypresses
	if err := emu.ula.StopRecording(); err != nil {
		t.Fatalf("rec stop: %v", err)
	}
	q := uint16(emu.mem.Read(balSym("qctr"))) | uint16(emu.mem.Read(balSym("qctr")+1))<<8
	fmt.Printf("MUSIC qctr=%d halted=%v pc=$%04X\n", q, emu.cpu.Halted, emu.cpu.PC)
	// content proof: DC-removed RMS must clear the silence floor
	raw, err := os.ReadFile(wavPath)
	if err != nil {
		t.Fatalf("missing %s: %v", wavPath, err)
	}
	// parse WAV PCM s16le, mono-fold
	var s []int16
	for i := 44; i+1 < len(raw); i += 4 { // stereo 44.1k: take L of each frame
		v := int16(uint16(raw[i]) | uint16(raw[i+1])<<8)
		s = append(s, v)
	}
	if len(s) == 0 {
		t.Fatalf("%s has no samples", wavPath)
	}
	dc := 0.0
	best := 0.0
	// sliding DC estimate
	win := 8192
	for k := 0; k < len(s); k += win {
		e := k + win
		if e > len(s) {
			e = len(s)
		}
		sum := 0.0
		for _, v := range s[k:e] {
			sum += float64(v)
		}
		dc = sum / float64(e-k)
		rs := 0.0
		for _, v := range s[k:e] {
			d := float64(v) - dc
			rs += d * d
		}
		rms := math.Sqrt(rs / float64(e-k))
		if rms > best {
			best = rms
		}
	}
	fmt.Printf("MUSIC wav=%d bytes bestRMS=%.1f\n", len(raw), best)
	if best < 50 {
		t.Fatalf("no AY content: best RMS %.1f — flat capture (attach quark?)", best)
	}
}
