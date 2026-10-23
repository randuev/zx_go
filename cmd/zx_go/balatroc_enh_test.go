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
	tapPath := os.Getenv("ZX_TAP")
	if tapPath == "" {
		tapPath = "/root/nerve-workspace/demos/balatroc-c/build/board.tap"
	}
	tapRaw, err := os.ReadFile(tapPath)
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
		if rd(hand) != 0xFF && rd(hand+1) != 0xFF &&
			rd(hand+4) != 0xFF && rd(handsL) > 0 {
			dealt = i
			break
		}
		if i%400 == 0 {
			stroke(4, 0x01, 1)
		}
		runF(1)
	}
	if dealt < 0 {
		t.Fatalf("round never dealt: hand[0]=$%02X hands_l=%d PC=$%04X flow=%d deal_upto=%d top=%d ndeck=%d",
			rd(hand), rd(handsL), emu.cpu.PC, rd(balSym("flow")), rd(balSym("deal_upto")), rd(balSym("top")), rd(balSym("ndeck")))
	}
	fmt.Printf("ENH gate: round dealt after %d frames, hands_l=%d\n", dealt, rd(handsL))
	// PASS 67 BADGE law v2 (Seva "all badges at the same time"):
	// DIRECT-PAINT census. The live score path puts the calculator's
	// number row on the SAME bytes as the badge rows — pixel census
	// there is a coin flip. Instead call the resident painter
	// card_cell($9F2F, sdcc_iy: args on stack, sentinel RET) into a
	// cleared scratch and demand EXACTLY ONE badge glyph: the badge
	// (6px at x+2) lives entirely in byte col bxc bits 5..0; byte col
	// bxc+1 must be ZERO (glyph6 spills px8+ only past a 6px glyph),
	// and bxc+2 may carry only the frame's right-edge bit0.
	{
		cc := balSym("card_cell")
		if cc == 0 || cc >= 0xC000 {
			t.Fatalf("card_cell not resident: $%04X", cc)
		}
		// capture live-loop state; RESTORE it after the census so the
		// round loop below keeps running exactly where it was
		sp0, pc0, im0 := emu.cpu.SP, emu.cpu.PC, emu.cpu.IM
		ifff0 := emu.cpu.IFF1
		eb := balSym("enh") // rodata neighbour probe: print ENHBADGE raw
		_ = eb
		var ebRaw [12]byte
		for i := range ebRaw {
			ebRaw[i] = emu.mem.Read(uint16(0xAA92) + uint16(i))
		}
		fmt.Printf("ENHBADGE@AA92 raw: %q core[0..3]=%02X%02X%02X%02X\n",
			string(ebRaw[:]), emu.mem.Read(0x8000), emu.mem.Read(0x8001),
			emu.mem.Read(0x8002), emu.mem.Read(0x8003))
		pa := func(y, bx int) byte {
			return emu.mem.Read(uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx))
		}
		for e := 0; e <= 7; e++ {
			x := 96
			y0 := 9 * 8
			// clear the cell span first
			for y := y0; y <= y0+39; y++ {
				for bx := 12; bx <= 15; bx++ {
					emu.mem.Write(uint16(0x4000+((y&7)<<8)+((y&0x38)<<2)+(bx)), 0x00)
				}
			}
			emu.cpu.Halted = false
			emu.cpu.IFF1 = false // diagnostics: no IRQ stack traffic mid-paint
			balBankSel(t, emu, 2)
			base := uint16(0xBF00)
			vals := []uint16{0x7D00, 9, uint16(x), 24, 24, 0, 1, 0, uint16(e)}
			for k, v := range vals {
				emu.mem.Write(base+uint16(2*k), byte(v))
				emu.mem.Write(base+uint16(2*k+1), byte(v>>8))
			}
			emu.cpu.SP = base
			emu.cpu.PC = cc
			g := 0
			for emu.cpu.PC != 0x7D00 {
				emu.cpu.StepInstructionWithIRQ()
				if g++; g > 2_000_000 {
					t.Fatalf("card_cell runaway (enh=%d)", e)
				}
			}
			fmt.Printf("  direct e=%d steps=%d PC=%04X SP=%04X\n", e, g, emu.cpu.PC, emu.cpu.SP)
			var after [12]byte
			for i := range after {
				after[i] = emu.mem.Read(uint16(0xAA92) + uint16(i))
			}
			fmt.Printf("  ENHBADGE after call: %q\n", string(after[:]))
			for y := y0; y <= y0+39; y++ {
				fmt.Printf("   y%d: c12=%02X c13=%02X c14=%02X\n", y, pa(y, 12), pa(y, 13), pa(y, 14))
			}
			badge, mid, right := 0, 0, 0
			for y := y0 + 32; y <= y0 + 38; y++ {
				badge += int(pa(y, 12) & 0x3F)
				mid += int(pa(y, 13))
				right += int(pa(y, 14) & 0xFE)
			}
			// (e<6 overflow diagnostics dumped above by the y-walk prints)
			if e >= 1 && e <= 6 {
				if badge == 0 {
					t.Errorf("enh=%d: badge glyph missing at bottom-left", e)
				}
				if mid != 0 || right != 0 {
					t.Errorf("enh=%d: ENHBADGE OVERFLOW — badge painted cols 13/14 (mid=%d right=%d): one glyph must fit byte col 12", e, mid, right)
				}
			} else if badge != 0 || mid != 0 || right != 0 {
				t.Errorf("enh=%d: NO badge expected outside 1..6 — cols dirty", e)
			}
		}
		savePNGAtomic("/tmp/enh_badge_direct.png", emu.renderFrame())
		// RESTORE the live round loop (bank 0 = PAGE GAME, IM2, IFF1)
		balBankSel(t, emu, 0)
		emu.cpu.SP, emu.cpu.PC, emu.cpu.IM = sp0, pc0, im0
		emu.cpu.IFF1 = ifff0
		emu.cpu.Halted = false
	}

	// live scoring law below (unchanged): PLAY commits the exact score.
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
		// PASS 67: live badge pixel census REMOVED — the calculator's
		// number row lives on these same bytes mid-score, so a pixel
		// census here is a coin flip (pass-66 false PASS was noise).
		// The DIRECT-PAINT census is the badge law now.
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
	runF(90)
	savePNGAtomic("/tmp/enh_stone.png", emu.renderFrame())

	fmt.Println("PASS ENH LAW: BONUS +30 / MULT +4 / GLASS x2 / STONE +50-exempt / WILD flush — all exact")
}
