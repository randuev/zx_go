package main

// balatroc_ante5win_test.go — SEVA shots 38/39 (2026-10-18 21:25): at
// ANTE 5 a SINGLE card play printed "BLIND DEFEATED!" — targets 11250
// and 45000 shown on the HUD while round score was 1440/450. Root cause
// pass 46d: compute_target's ante>=5 branch stringified tgt IN PLACE
// (tgt /= 10 down to ZERO) — HUD digits survived in the buffer, the
// live target died, score >= 0 always won.
// LAW: after compute_target at any state, tgt must equal the printed
// number. This test: boot + START, force ante=5/blind=0, CALL the REAL
// compute_target ($B0EC), demand tgt==11250 (not 0!), then play a tiny
// hand through the REAL PLAY key and demand NO STR_WIN, tgt still live.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocAnte5WinPlus2(t *testing.T) {
	if os.Getenv("ANTE5") == "" {
		t.Skip("set ANTE5=1")
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
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	const ante = 0xF7EE
	const blind = 0xFA21
	const tgt = 0xF7F0
	const score = 0xFA19
	const handsL = 0xF7EC
	const msg = 0xFA22
	const STRWIN = 0xDACA
	const computeTarget = 0xB0EC
	rd16 := func(addr uint16) uint32 {
		return uint32(emu.mem.Read(addr)) | uint32(emu.mem.Read(addr+1))<<8 |
			uint32(emu.mem.Read(addr+2))<<16 | uint32(emu.mem.Read(addr+3))<<24
	}
	emu.paused.Store(false)
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
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = 0xAA03 // title_screen pass-47
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	runF(120) // title
	stroke(4, 0x01, 150) // START -> first play screen
	// force the reported state: ante 5, SMALL blind (shot 38)
	emu.mem.Write(ante, 5)
	emu.mem.Write(blind, 0)
	// CALL the REAL compute_target and watch tgt survive
	hlret := uint16(0)
	savedSP := emu.cpu.SP
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.Halted = false // title-screen RET parked us on the $7D00 HALT
	emu.cpu.PC = computeTarget
	stepsRun := 0
	for i := 0; i < 400000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		stepsRun = i
		if emu.cpu.PC == sent {
			hlret = emu.cpu.HL()
			break
		}
	}
	fmt.Printf("steps=%d halted=%v paused=%v PC=$%04X SP=$%04X IFF1=%v\n",
		stepsRun, emu.cpu.Halted, emu.paused.Load(), emu.cpu.PC, emu.cpu.SP, emu.cpu.IFF1)
	emu.cpu.SP = savedSP
	tg := rd16(tgt)
	fmt.Printf("compute_target ante=5 small: tgt=%d (want 11250, NOT 0) hlret=$%04X\n", tg, hlret)
	if tg != 11250 {
		t.Fatalf("compute_target ante5 small corrupts target: tgt=%d — win at score>=0 is guaranteed", tg)
	}
	// full play path: one card selected, PLAY — must NOT print STR_WIN
	for j := 0; j < 4; j++ {
		emu.mem.Write(score+uint16(j), byte(uint(1440)>>uint(8*j)))
	}
	// RIGHT x0: cursor already on a card; select exactly one, PLAY
	stroke(4, 0x08, 40) // SEL
	emu.kbd.PressMatrixKey(4, 0x01, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	winLie := false
	for f := 0; f < 700; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		m := uint16(emu.mem.Read(msg)) | uint16(emu.mem.Read(msg+1))<<8
		if m == STRWIN {
			fmt.Printf("!! STR_WIN at frame %d: score=%d tgt=%d\n", f, rd16(score), rd16(tgt))
			winLie = rd16(score) < rd16(tgt)
			break
		}
	}
	fmt.Printf("post-play ante5: score=%d tgt=%d hands_l=%d PC=$%04X\n",
		rd16(score), rd16(tgt), emu.mem.Read(handsL), emu.cpu.PC)
	if winLie {
		t.Fatalf("FALSE WIN at ante5: score=%d tgt=%d", rd16(score), rd16(tgt))
	}
	if tg := rd16(tgt); tg == 0 {
		t.Fatalf("tgt destroyed to 0 again: %d", tg)
	}
	fmt.Println("ante5 target lives; no false win")
}
