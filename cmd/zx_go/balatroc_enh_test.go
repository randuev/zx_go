package main

// balatroc_enh_test.go — PASS 64 ENHANCED CARDS LAW GATE (ENH=1).
//
// The SCORE must follow the enhanced-card law EXACTLY:
//   BONUS  +30 chips per played card
//   MULT   +4 mult per played card
//   GLASS  x2 mult per played card
//   STONE  +50 chips, scores but is EXCLUDED from the hand-type test
//          (3 stones + 2 unpaired hearts = HIGH CARD, not a pair)
//   WILD   completes a flush of four hearts and is never debuffed
//          (dsuit=clubs poked — the wild-club still flush-scores)
//
// Honest-path law: boot through crt0 ($8000) exactly like the tape
// loader's RANDOMIZE USR, START to a real round, then drive each law:
// poke a known hand + enh pattern, stroke PLAY ('0'), require the
// committed score to equal the law's arithmetic. The pokes are test
// scaffolding; the whole scoring path is game code.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocEnhLawPlus2(t *testing.T) {
	if os.Getenv("ENH") == "" {
		t.Skip("set ENH=1")
	}
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Skipf("no balatroc tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var coreAddr uint16
	var core, music []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			coreAddr, core = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			music = blocks[i+1].data
		}
	}
	if core == nil {
		t.Fatal("no core block in board.tap")
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
	enh := balSym("enh")
	hand := balSym("hand")
	sel := balSym("selected")
	selo := balSym("sel_order")
	norder := balSym("n_order")
	score := balSym("score")
	tgt := balSym("tgt")
	handsL := balSym("hands_l")
	dsuit := balSym("dsuit")
	for _, pair := range []struct {
		n string
		a uint16
	}{{"enh", enh}, {"hand", hand}, {"selected", sel}, {"sel_order", selo},
		{"n_order", norder}, {"score", score}, {"dsuit", dsuit}} {
		if pair.a == 0 {
			t.Fatalf("missing symbol %s", pair.n)
		}
	}
	w := func(a uint16, v byte) { emu.mem.Write(a, v) }
	rd := func(a uint16) byte { return emu.mem.Read(a) }
	rd32 := func(a uint16) uint32 {
		return uint32(emu.mem.Read(a)) | uint32(emu.mem.Read(a+1))<<8 |
			uint32(emu.mem.Read(a+2))<<16 | uint32(emu.mem.Read(a+3))<<24
	}
	w32 := func(a uint16, v uint32) {
		for j := 0; j < 4; j++ {
			emu.mem.Write(a+uint16(j), byte(v>>uint(8*j)))
		}
	}
	htype := balSym("htype")
	lastN := balSym("last_n")
	playA := balSym("play")

	emu.paused.Store(false)
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

	runF(220)
	for i, v := range core {
		emu.mem.Write(coreAddr+uint16(i), v)
	}
	if music != nil {
		for i, v := range music {
			emu.mem.Write(0x6000+uint16(i), v)
		}
	}
	balRingPreload(t, emu, tapRaw)

	sent := uint16(0x7D00)
	for i := sent; i < sent+8; i++ {
		w(i, 0x76)
	}
	emu.cpu.SP = 0xBFF5
	w(0xBFF5, byte(sent))
	w(0xBFF6, byte(sent>>8))
	emu.cpu.PC = 0x8000 // crt0 — the ONLY honest door
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF(30)

	// START into round 1 (splash may eat the first stroke)
	stroke(4, 0x01, 120)
	stroke(4, 0x01, 120)
	dealt := -1
	for i := 0; i < 14000; i++ {
		if rd(hand) != 0 && rd(hand) != 0xFF && rd(hand+1) != 0 &&
			rd(hand+4) != 0 && rd(hand+4) != 0xFF && rd(handsL) > 0 {
			dealt = i
			break
		}
		if i%400 == 0 {
			stroke(4, 0x01, 1)
		}
		runF(1)
	}
	if dealt < 0 {
		t.Fatalf("round never dealt: hand[0]=$%02X hands_l=%d PC=$%04X",
			rd(hand), rd(handsL), emu.cpu.PC)
	}
	fmt.Printf("ENH gate: round dealt after %d frames, hands_l=%d\n", dealt, rd(handsL))
	w32(tgt, 999999) // no accidental blind win mid-gate
	w(handsL, 5)

	// clear all enhancements, then set a known 5-card hand.
	// cards[i] = card value, enh[i] = ENH_* code, marks all 5 played.
	set := func(cards []byte, enhs []byte, bossDSuit byte) {
		for i := 0; i < 52; i++ {
			w(enh+uint16(i), 0)
		}
		for i := 0; i < 8; i++ {
			v := byte(0)
			if i < len(cards) {
				v = 1 // selected[] is PER-SLOT bytes, not a bitmask
				if i < len(enhs) {
					w(enh+uint16(cards[i]), enhs[i])
				}
				w(selo+uint16(i), cards[i]) // u8: CARDS in selection order
			}
			w(sel+uint16(i), v)
		}
		for i := 0; i < 8; i++ {
			v := byte(0xFF)
			if i < len(cards) {
				v = cards[i]
			}
			w(hand+uint16(i), v)
		}
		w(norder, byte(len(cards)))
		w(dsuit, bossDSuit)
	}
	law := func(t *testing.T, name string, exp uint32) {
		t.Helper()
		w(handsL, 9) // every EV_PLAY decrements it — top up per case
		w32(score, 0)
		// press #1: EV_PLAY — play_sel() commits the score SYNCHRONOUSLY,
		// long before score_anim/score_wait are reached.
		emu.kbd.PressMatrixKey(4, 0x01, true)
		runF(4)
		emu.kbd.PressMatrixKey(4, 0x01, false)
		// Robust stroke law: score_anim()/score_wait() CONSUME presses
		// (their snap + the final `do{}while(scanKey()==EV_NONE)`), so a
		// single press can be swallowed depending on ramp timing. Retry a
		// fresh PLAY every ~40 frames until the committed score lands, then
		// release: re-press until clear_play() empties the play row — the
		// observable proof the game returned to the round loop.
		ok := false
		var lastPC uint16
		for i := 0; i < 30000; i++ {
			runF(1)
			s := rd32(score)
			if s == exp {
				ok = true
				lastPC = emu.cpu.PC
				break
			}
			if s > exp {
				break
			}
			if i%40 == 39 && rd(playA) == 0xFF {
				emu.kbd.PressMatrixKey(4, 0x01, true)
				runF(2)
				emu.kbd.PressMatrixKey(4, 0x01, false)
			}
		}
		if !ok {
			var hb [8]byte
			for j := 0; j < 8; j++ {
				hb[j] = rd(hand + uint16(j))
			}
			t.Fatalf("law %s: score=%d want %d PC=$%04X handsL=%d play0=%d htype=%d lastN=%d hand=%v",
				name, rd32(score), exp, emu.cpu.PC, rd(handsL), rd(playA), rd(htype), rd(lastN), hb)
		}
		cleared := false
		for i := 0; i < 20000; i++ {
			runF(1)
			if rd(playA) == 0xFF {
				cleared = true
				break
			}
			if i%40 == 39 {
				emu.kbd.PressMatrixKey(4, 0x01, true)
				runF(2)
				emu.kbd.PressMatrixKey(4, 0x01, false)
			}
		}
		if !cleared {
			t.Fatalf("law %s: play row never cleared (score=%d PC=$%04X)", name, rd32(score), emu.cpu.PC)
		}
		runF(60) // glass_break/banners/board settle before the next poke
		fmt.Printf("  %-12s score=%d OK (release @PC=$%04X)\n", name, exp, lastPC)
	}

	// PAIR of twos baseline: (10 + 2+2) * 2 = 28
	set([]byte{0, 13}, nil, 0xFF)
	fmt.Printf("pre-PLAY: hand=%v sel=%02X norder=%d n=%d\n",
		func() [8]byte { var b [8]byte; for i := range b {
			b[i] = rd(hand + uint16(i))
		}; return b }(), rd(sel), rd(norder), rd(lastN))
	law(t, "PAIR base", 28)

	// BONUS x2: +60 chips -> (10+4+60)*2 = 148
	set([]byte{0, 13}, []byte{1, 1}, 0xFF)
	law(t, "BONUS", 148)

	// MULT x2: +4 mult each -> (10+4)*10 = 140
	set([]byte{0, 13}, []byte{2, 2}, 0xFF)
	law(t, "MULT", 140)

	// GLASS x2: mult 2*2*2 -> (10+4)*8 = 112
	set([]byte{0, 13}, []byte{6, 6}, 0xFF)
	law(t, "GLASS", 112)

	// WILD flush: 4 hearts + wild CLUB, boss de-buffs CLUBS (dsuit=2):
	// wild never debuffed, flush completes -> (35 + 2+3+4+5+10) * 4 = 236
	set([]byte{13, 14, 15, 16, 48}, []byte{0, 0, 0, 0, 3}, 2)
	law(t, "WILD flush", 236)

	// STONE: 3 stones + 2 unpaired hearts: EXCLUDED from the type test
	// (no pair among stones) -> HIGH CARD (chip 5, mult 1); the HIGH mask
	// rides the real highest card 3♥ (+3) — never a stone — and each
	// stone scores +50 -> (5 + 3 + 150) * 1 = 158
	set([]byte{13, 14, 44, 45, 46}, []byte{0, 0, 4, 4, 4}, 0xFF)
	law(t, "STONE high", 158)

	fmt.Println("PASS ENH LAW: BONUS +30 / MULT +4 / GLASS x2 / STONE +50-exempt / WILD flush — all exact")
}
