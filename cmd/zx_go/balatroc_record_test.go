package main

// balatroc_record_test.go — SEVA 2026-10-19: "play these sounds in isolation,
// record them, iterate until they're right".
//
// Runs the REAL game exactly like the real +2: blocks direct-loaded, jump to
// the CRT entry $8000 (NOT title_screen mid-function — that was the harness
// artifact: START made the whole chain RET onto the fake sentinel HALT and
// the CPU froze at $7D01 while on real hardware main()'s loop keeps running).
// One continuous WAV of the whole round at fixed guest-time segments
// (frame = 20 ms):
//
//	 t= 0.0 s  title music
//	 t= 6.0 s  START stroke -> deal
//	 t= 9.0 s  idle play screen (Seva's video window)
//	 t=15.0 s  cursor RIGHT (chirp)
//	 t=18.0 s  SEL card
//	 t=21.0 s  PLAY hand -> score tally + blind result
//	 t=42.0 s  NEXT -> new round deal (shuffle)
//
// PASS: file exists and the CPU is alive (not halted) at the end; the WAV is
// then analysed offline for per-event pitch/cadence so sound tables in
// music.c can be tuned against the capture.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
	"github.com/conorarmstrong/zx_go/pkg/ula"
)

func TestBalatrocIdleTempoPlus2(t *testing.T) {
	if os.Getenv("IDLETEMPO") == "" { t.Skip("set IDLETEMPO=1") }
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
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil { t.Fatal(err) }
	emu.paused.Store(true)
	for i := 0; i < 220; i++ { runOneFrameHeadless(emu, roms.ModelPlus2) }
	for i, v := range bd { emu.mem.Write(a+uint16(i), v) }
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
	// full frames incl. Pump? record path irrelevant; just count quark per frame at TITLE
	q := func() int { return int(emu.mem.Read(0xFCD2)) | int(emu.mem.Read(0xFA2C))<<8 }
	runF := func(n int) { for i := 0; i < n; i++ { runOneFrameHeadless(emu, roms.ModelPlus2) } }
	runF(120)
	q0 := q()
	runF(100)
	q1 := q()
	fmt.Printf("IDLE quark rate = %.2f per frame (delta %d over 100f)\n", float64(q1-q0)/100.0, q1-q0)
}

func TestBalatrocRecordSoundsPlus2(t *testing.T) {
	if os.Getenv("RECSND") == "" {
		t.Skip("set RECSND=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd, md []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			md = blocks[i+1].data
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.headless = true
	nf.recordAudio = "/tmp/bal_real.wav"
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	// THE attach law: StartRecording is a silent no-op unless u.audio exists.
	// The headless binary calls EnableAudioSilent() before recording; the
	// harness must do the same or it records NOTHING (flat-rail "WAV" was a
	// stale file from an earlier run — IsRecording()==false was the tell).
	if emu.ula == nil {
		t.Fatal("no ula")
	}
	emu.ula.EnableAudioSilent()
	emu.paused.Store(true)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	for i, v := range md {
		emu.mem.Write(0x6000+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFFE
	emu.mem.Write(0xFFFC, byte(sent))
	emu.mem.Write(0xFFFD, byte(sent>>8))
	emu.cpu.PC = 0x8000 // CRT cold start — the true RANDOMIZE USR contract
	emu.cpu.Halted = false
	emu.cpu.D, emu.cpu.E, emu.cpu.H, emu.cpu.L = 0, 0, 0x80, 0x00 // DEHL=$00008000 (RANDOMIZE USR arg) — crt stacks via HL
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
			emu.ula.PumpAudioFrame()
		}
	}
	ph := func(tag string) {
		qc := uint16(emu.mem.Read(0xFCD2)) | uint16(emu.mem.Read(0xFA2C))<<8
		sc := int32(uint(emu.mem.Read(0xFC35)) | uint(emu.mem.Read(0xFEDE))<<8 |
			uint(emu.mem.Read(0xFC37))<<16 | uint(emu.mem.Read(0xFEE0))<<24)
		fmt.Printf("PHASE %-22s PC=$%04X SP=$%04X halted=%v qctr=%d score=%d\n",
			tag, emu.cpu.PC, emu.cpu.SP, emu.cpu.Halted, qc, sc)
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	runF(150) // CRT init + main loop + title paint
	ph("title-painted")
	if err := emu.ula.StartRecording("/tmp/bal_real.wav"); err != nil {
		fmt.Println("rec start:", err)
	}
	runF(300) // 0.0-6.0 s: title music
	ph("title@6s")
	stroke(4, 0x10, 150) // START (K) -> first deal (6.0-9.0 s)
	ph("after-START")
	runF(300) // 9.0-15.0 s idle play (Seva's video window)
	ph("play-idle@15s")
	stroke(4, 0x04, 150) // 15.0 s cursor RIGHT (sfx_right)
	ph("after-right")
	stroke(4, 0x08, 150) // 18.0 s SEL card (sfx_sel)
	ph("after-sel")
	// make the hand winnable so STR_WIN fires: score above target
	for j := 0; j < 4; j++ {
		emu.mem.Write(0xFC35+uint16(j), byte(uint(300)>>uint(8*j)))
	}
	stroke(4, 0x01, 500) // 21.0 s PLAY hand (0) -> scoring tally + blind result
	ph("after-play@31s")
	runF(250) // blind banner / NEXT prompt settles
	stroke(4, 0x10, 300) // NEXT (K) -> new round deal (shuffle)
	ph("after-next@42s")
	if err := emu.ula.StopRecording(); err != nil {
		fmt.Println("rec stop:", err)
	}
	fmt.Printf("AUDIO pumps=%d skips=%d pushes=%d speaker=%v\n", ula.PumpDebugPumps(), ula.PumpDebugSkips(), ula.PumpDebugPushes(), emu.ula.Speaker)
	st, err2 := os.Stat("/tmp/bal_real.wav")
	if err2 != nil {
		t.Fatalf("MISSING /tmp/bal_real.wav: %v", err2)
	}
	fmt.Printf("recorded /tmp/bal_real.wav %d bytes\n", st.Size())
	if emu.cpu.Halted {
		t.Fatalf("CPU parked halted at $%04X mid-session", emu.cpu.PC)
	}
}
