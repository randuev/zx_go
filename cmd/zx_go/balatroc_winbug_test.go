package main

// balatroc_winbug_test.go — SEVA report: "what did i beat the blind if
// my score has not reached the needed score" (1196/90000). 90000 is the
// legit ante-5 BOSS target (TGTU32[14]). Repro: real boot + START, then
// force tgt=90000, select cards, PLAY a tiny hand. PASS = honest engine
// (no win while score<tgt). WITNESS = every write to tgt/score with the
// writer PC, plus the msg change to STR_WIN. The HUD paints tgt_s (a
// stale string snapshot); only live tgt governs the win — the hook proves
// who lies.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocWinBugPlus2(t *testing.T) {
	if os.Getenv("WINBUG") == "" {
		t.Skip("set WINBUG=1")
	}
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var boardAddr uint16
	var boardData, musicData []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			boardAddr, boardData = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			musicData = blocks[i+1].data
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
		t.Fatalf("newEmulator: %v", err)
	}
	const score = 0xFC37
	const tgt = 0xF9FD
	const msg = 0xFB89
	const handsL = 0xF9F9
	const ante = 0xF9FB
	const blind = 0xFEE4
	const STRWIN = 0xD8AB
	rd16 := func(a uint16) uint32 {
		return uint32(emu.mem.Read(a)) | uint32(emu.mem.Read(a+1))<<8 |
			uint32(emu.mem.Read(a+2))<<16 | uint32(emu.mem.Read(a+3))<<24
	}
	type wrEv struct {
		pc   uint16
		addr uint16
		val  byte
	}
	var tgtWr, scoreWr []wrEv
	emu.mem.SetAllWriteHook(func(addr uint16, val byte) {
		pc := emu.cpu.PC
		switch {
		case addr >= tgt && addr <= tgt+3:
			tgtWr = append(tgtWr, wrEv{pc, addr, val})
		case addr >= score && addr <= score+3:
			scoreWr = append(scoreWr, wrEv{pc, addr, val})
		}
	})
	defer emu.mem.SetAllWriteHook(nil)
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
balRingPreload(t, emu, tapRaw)
	for i, v := range musicData {
		emu.mem.Write(0x6000+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
	emu.cpu.PC = boardAddr
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
	fmt.Printf("round start: score=%d tgt=%d ante=%d blind=%d hands_l=%d\n",
		rd16(score), rd16(tgt), emu.mem.Read(ante), emu.mem.Read(blind), emu.mem.Read(handsL))
	// force the reported state: BOSS target 90000, score stays whatever
	emu.mem.Write(tgt, 0x90)
	emu.mem.Write(tgt+1, 0x5F)
	emu.mem.Write(tgt+2, 0x01)
	emu.mem.Write(tgt+3, 0x00)
	// PASS-46 32-bit bank test: start just under the u16 ceiling — if the
	// score bank spills garbage past 65535, score>=tgt can go TRUE while
	// the HUD (tgt_s string snapshot) still honestly reads "90000".
	emu.mem.Write(score, 0xFA) // 65530
	emu.mem.Write(score+1, 0xFF)
	emu.mem.Write(score+2, 0x00)
	emu.mem.Write(score+3, 0x00)
	tgtWr = tgtWr[:0] // ignore our own force-write
	// select a few cards then play: RIGHT x3, SEL x4, PLAY
	// PASS-46 diag: SEL tails must outlast scanKey release-latch (~1 fr
	// is enough, but the stroke edges are poll-sampled -> give air)
	sel := func() string {
		s := ""
		for i := 0; i < 8; i++ {
			s += fmt.Sprintf("%02X", emu.mem.Read(0xFECA+uint16(i)))
		}
		return s
	}
	for i := 0; i < 3; i++ {
		stroke(4, 0x04, 40)
		fmt.Printf("after RIGHT%d: cursor=%d msg=$%04X PC=$%04X\n",
			i, emu.mem.Read(0xFEDB),
			uint16(emu.mem.Read(msg))|uint16(emu.mem.Read(msg+1))<<8, emu.cpu.PC)
	}
	stroke(4, 0x08, 40) // exactly ONE SEL: cursor stays on a card
	fmt.Printf("selected=%s cursor=%d\n", sel(), emu.mem.Read(0xFEDB))
	scoreWr = scoreWr[:0] // ignore pre-play banks
	// PLAY and watch the check region frame by frame
	emu.kbd.PressMatrixKey(4, 0x01, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	winLie := false
	for f := 0; f < 600; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		m := uint16(emu.mem.Read(msg)) | uint16(emu.mem.Read(msg+1))<<8
		if m == STRWIN {
			fmt.Printf("!! WIN at frame %d: score=%d tgt=%d hands_l=%d PC=$%04X\n",
				f, rd16(score), rd16(tgt), emu.mem.Read(handsL), emu.cpu.PC)
			winLie = rd16(score) < rd16(tgt)
			break
		}
	}
	fmt.Printf("post-play: score=%d tgt=%d msg=$%04X hands_l=%d PC=$%04X\n",
		rd16(score), rd16(tgt),
		uint16(emu.mem.Read(msg))|uint16(emu.mem.Read(msg+1))<<8,
		emu.mem.Read(handsL), emu.cpu.PC)
	for i, w := range tgtWr {
		fmt.Printf("TGT-WR %d pc=$%04X addr=$%04X val=%02X\n", i, w.pc, w.addr, w.val)
	}
	for i, w := range scoreWr {
		if i < 24 {
			fmt.Printf("SCORE-WR %d pc=$%04X addr=$%04X val=%02X\n", i, w.pc, w.addr, w.val)
		}
	}
	if winLie {
		t.Fatalf("FALSE WIN: banner while score=%d < tgt=%d — see writer log", rd16(score), rd16(tgt))
	}
	if rd16(score) >= rd16(tgt) {
		t.Fatalf("repro failed: selected hand scored >= 90000 (score=%d) — pick smaller", rd16(score))
	}
	fmt.Println("honest engine on this path: no win below target")
}
