package main

// balatroc_qtempo_test.go — PASS 48f IM2 TEMPO GATE (Seva 2026-10-19:
// "music still slows down severely during redraw" — the THIRD time). The
// quark is no longer poll-driven AT ALL: _music_boot installs an IM2 ISR
// (vector page $59) that ticks _tick at exactly one quark per vsync.
// This gate measures the quark/frame ratio through the three hottest
// repaint moments in the game — moments that stretched the poll player to
// 2-5x its tempo (Seva's ears on hardware, harness logs agreeing):
//   IDLE   : main loop spinning light
//   DEAL   : shuffle+deal repaint (the ante-5 hill)
//   SHOP   : shop_screen entry + its animation
// LAW: quark delta / frame delta >= 0.90 through EVERY phase. A poll
// player fails this at DEAL/SHOP; an ISR player cannot (hardware fact).
// Harness constants from board.map of tap 888f1d46…:
// _qctr=$FCD2 _mptr=$FCCD _music_boot=$0xd58a _shop_screen=$0xa496
// _shuffle=$0xb102 _deal=$0xb172 _tgt=$FCA5 _score=$FEDD.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocQTempoPlus2(t *testing.T) {
	if os.Getenv("QTEMPO") == "" {
		t.Skip("set QTEMPO=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd, md []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			md = blocks[i+1].data
		}
	}
	if bd == nil {
		t.Fatal("no board block")
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator: %v", err)
	}
	const qctr = 0xfcd4
	const musicBoot = 0xd58a
	const shopScreen = 0xa496
	const shuffle = 0xb102
	const deal = 0xb172
	emu.paused.Store(false)
	for i := 0; i < 200; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	if md != nil {
		for i, v := range md {
			emu.mem.Write(0x6000+uint16(i), v)
		}
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = musicBoot
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	stepN := func(n int) {
		for i := 0; i < n; i++ {
			if emu.cpu.Halted {
				emu.cpu.Halted = false
			}
			emu.cpu.StepInstructionWithIRQ()
		}
	}
	stepN(4000) // install IM2: vectab fill + im 2 + ei
	if im := emu.cpu.IM; im != 2 {
		t.Fatalf("music_boot left IM=%d, want 2", im)
	}
	fmt.Printf("IM2 installed: I=$%02X IM=%d\n", emu.cpu.I, emu.cpu.IM)
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	q := func() int {
		return int(emu.mem.Read(qctr)) | int(emu.mem.Read(qctr+1))<<8
	}
	// ---- phase ratio helper: jump PC, run N frames, quark must keep 50 Hz
	phase := func(name string, pc uint16, n int) float64 {
		emu.mem.Write(sent, 0x76)
		emu.cpu.SP = 0xFF56
		emu.mem.Write(0xFF56, byte(sent))
		emu.mem.Write(0xFF57, byte(sent>>8))
		emu.cpu.PC = pc
		emu.cpu.Halted = false
		q0 := q()
		runF(n)
		q1 := q()
		dq := q1 - q0
		if q1 < q0 {
			dq = (0x10000 + q1) - q0 // odometer wrap
		}
		r := float64(dq) / float64(n)
		fmt.Printf("%-6s frames=%d quarks=%d ratio=%.2f PC=$%04X\n",
			name, n, dq, r, emu.cpu.PC)
		if r < 0.90 {
			t.Fatalf("%s tempo STRETCHED: %d quarks / %d frames = %.2f",
				name, dq, n, r)
		}
		return r
	}
	// IDLE: main poll loop (light repaint steady state)
	phase("main", 0xab4d, 100)
	// DEAL: shuffle + deal repaint through the real game path
	phase("deal", deal, 100)
	// SHOP: shop_screen entry + animation frames
	phase("shop", shopScreen, 100)
	// and finally a shuffle call - the hottest paint on the hill
	phase("shuffle", shuffle, 100)
	fmt.Println("IM2 tempo exact through deal/shop/shuffle: PASS")
}
