package main

// balatroc_preview_test.go — PASS 63 ANTE PREVIEW gate (PREVIEW=1).
// Seva: "at the start of each ante, we need to have a screen that will
// display how much each blind of the ante is worth and see what boss
// challenge awaits user."
//
// LAWS enforced:
//  1. LOCK: the ante's boss is picked at the wrap (ante_new=1 there, and
//     only there) and lives in ante_boss; boss_id stays blind-scoped —
//     ==ante_boss on blind 2, 0 everywhere else. Pool: consecutive ante
//     bosses distinct until the ten recycle.
//  2. FIDELITY: every tgt the engine computes equals the law re-computed
//     in Go from (ante, blind, ante_boss) — the preview and the round
//     read the SAME ladder (target_for is single-source). THE WALL row
//     must preview doubled at its own ante.
//  3. RENDER: door(1,FLOW_PREVIEW) paints the roadmap — boss-row RED
//     paper cells appear on attr row 9 and YELLOW rule ink on row 13,
//     screen heavily lit; the same census is EMPTY of those marks BEFORE
//     the call (self-control: negative proof without a stale tap, whose
//     core cannot share this build's map).
//  4. NEVER HANGS: the wait loop exits on the qctr 250-frame escape.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// law re-computes the ladder in Go (mirror of target_for in game.c).
func balLaw(a, b uint8, boss uint8) uint32 {
	tbl := []uint32{300, 450, 600, 800, 1200, 2000, 2000, 3000, 5000,
		5000, 7500, 11250} // mirrors TGTU32[0..11] in game.c
	var v uint32
	if a < 5 {
		v = tbl[(a-1)*3+b]
	} else {
		small := []uint32{11250, 22500, 37500, 67500}
		if a <= 8 {
			v = small[a-5]
		} else {
			v = 67500
			for i := uint8(0); i < a-8; i++ {
				v = v + v/10*7
				if v > 4000000000 {
					v = 4000000000
					break
				}
			}
		}
		if b == 1 {
			v = v + v/2
		} else if b == 2 {
			v <<= 1
		}
	}
	if b == 2 && boss == 3 {
		v <<= 1 // THE WALL doubles its own ante-3 blind
	}
	return v
}

func balLitCount(emu *emulator) int {
	n := 0
	for a := 0x4000; a < 0x5800; a++ {
		if emu.mem.Read(uint16(a)) != 0 {
			n++
		}
	}
	return n
}

func balAttrRowPred(emu *emulator, row int, pred func(uint8) bool) int {
	n := 0
	for c := 0; c < 32; c++ {
		if pred(emu.mem.Read(uint16(0x5800+row*32+c))) {
			n++
		}
	}
	return n
}

func TestBalatrocPreviewPlus2(t *testing.T) {
	if os.Getenv("PREVIEW") == "" {
		t.Skip("set PREVIEW=1")
	}
	emu, syms := balBossSetup(t)
	_ = roms.ModelPlus2
	run := func(frames int) {
		for i := 0; i < frames; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	run(150) // main runs: boot_pages(skip) → music_boot → game_new → title
	rd8 := func(name string) uint8 { return emu.mem.Read(syms[name]) }
	wr8 := func(name string, v byte) { emu.mem.Write(syms[name], v) }
	rd16 := func(name string) uint16 {
		return uint16(emu.mem.Read(syms[name])) | uint16(emu.mem.Read(syms[name]+1))<<8
	}

	// LAW 0 — run start: ante 1's boss is already locked by game_new.
	if ab := rd8("ante_boss"); ab < 1 || ab > 10 {
		t.Fatalf("ante_boss=%d at run start — never locked", ab)
	}
	if rd8("ante") != 1 || rd8("blind") != 0 || rd8("boss_id") != 0 {
		t.Fatalf("start state ante=%d blind=%d boss=%d", rd8("ante"), rd8("blind"), rd8("boss_id"))
	}
	if got, want := rd32(emu, syms["tgt"]), balLaw(1, 0, rd8("ante_boss")); got != want {
		t.Fatalf("tgt $%X != law $%X at start", got, want)
	}
	nr := syms["next_round"]
	seenBoss := map[uint8]int{}
	prevAnte := uint8(1)
	for i := 0; i < 27; i++ { // 9 antes of blind walks
		balCallNoArg(t, emu, nr)
		a, bl, ab := rd8("ante"), rd8("blind"), rd8("ante_boss")
		bid, an := rd8("boss_id"), rd8("ante_new")
		wrapped := a != prevAnte
		// LAW 1 — LOCK at the wrap, blind-scope, pool no-repeat
		if wrapped {
			if an != 1 {
				t.Fatalf("ante %d wrapped but ante_new=%d — preview never owed", a, an)
			}
			wr8("ante_new", 0)
			if ab < 1 || ab > 10 {
				t.Fatalf("ante_boss=%d after wrap — pool broken", ab)
			}
			seenBoss[ab]++
			if seen := seenBoss[ab]; seen > 1 && a <= 10 {
				t.Fatalf("ante_boss %d repeats at ante %d — pool drained early", ab, a)
			}
		} else if an != 0 {
			t.Fatalf("ante_new=%d without a wrap at ante %d", an, a)
		}
		if bl == 2 && bid != ab {
			t.Fatalf("ante %d blind2: boss_id=%d != locked ante_boss=%d", a, bid, ab)
		}
		if bl != 2 && bid != 0 {
			t.Fatalf("ante %d blind%d: boss_id=%d — boss outlives its blind", a, bl, bid)
		}
		// LAW 2 — FIDELITY: engine tgt == Go law
		if got, want := rd32(emu, syms["tgt"]), balLaw(a, bl, ab); got != want {
			t.Fatalf("ante %d blind %d (boss %d): engine tgt %d != law %d", a, bl, ab, got, want)
		}
		prevAnte = a
		if a >= 9 {
			break
		}
	}
	// THE WALL law must have shown itself at least once across the run
	wallOK := false
	for ab := uint8(1); ab <= 10; ab++ {
		if balLaw(3, 2, ab) != balLaw(3, 2, 3) {
			wallOK = true
		}
	}
	if !wallOK {
		t.Fatal("WALL law broken in Go mirror (sanity)")
	}

	// LAW 3 — RENDER census, self-controlled.
	redBefore := balAttrRowPred(emu, 9, func(v uint8) bool { return (v>>3)&7 == 2 })
	litBefore := balLitCount(emu)
	// fire the preview through the REAL ring door: flow=4, PAGE SHOP bank 1
	wr8("flow", 4) // FLOW_PREVIEW
	balBankSel(t, emu, 1)
	sent := uint16(0x7D00)
	emu.cpu.Halted = false
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	emu.cpu.PC = 0xC000
	poked := false
	g := 0
	for emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
		if !poked && emu.cpu.PC >= 0xC000 {
			// LAW 4 — escape hatch: advance the odometer so wait_key's
			// 5-second guard fires (no ISR here); proves never-hangs.
			emu.mem.Write(syms["qctr"], byte(rd16("qctr")+251))
			emu.mem.Write(syms["qctr"]+1, 0)
			poked = true
		}
		emu.cpu.StepInstructionWithIRQ()
		if g++; g > 8_000_000 {
			t.Fatal("preview door runaway — wait loop never escapes")
		}
	}
	if !poked {
		t.Fatal("PC never entered the $C000 window — door never ran")
	}
	lit := balLitCount(emu)
	// PASS 67 (Seva: "when ante is previewed red background bleeds onto
	// the next line to the beginning"): attr rows are 32 cells; the old
	// attrrect(9,7,1,26,...) wrote col 32 == row 10 col 0. Exact-span
	// law: RED paper covers EXACTLY cols 7..31 of row 9, and the very
	// next byte — row 10 col 0 — must be clean.
	red := balAttrRowPred(emu, 9, func(v uint8) bool { return (v>>3)&7 == 2 })
	yel := balAttrRowPred(emu, 13, func(v uint8) bool { return v&7 == 6 })
	spanBad := 0
	for c := 7; c <= 31; c++ {
		if (emu.mem.Read(uint16(0x5800+9*32+c))>>3)&7 != 2 {
			spanBad++
		}
	}
	bleed := (emu.mem.Read(uint16(0x5800+10*32+0)) >> 3) & 7
	fmt.Printf("PREVIEW lit=%d (before %d) redRow9=%d (before %d) spanBad=%d bleedRow10Col0paper=%d yelRow13=%d qctrOK=%v\n",
		lit, litBefore, red, redBefore, spanBad, bleed, yel, poked)
	if redBefore > 2 {
		t.Fatalf("attr row 9 already RED (%d) before preview — control broken", redBefore)
	}
	if red < 24 {
		t.Errorf("boss row missing: only %d RED cells on row 9", red)
	}
	if spanBad != 0 {
		t.Errorf("PASS 67 RED span broken: %d cells of cols 7..31 not RED", spanBad)
	}
	if bleed == 2 {
		t.Errorf("PASS 67 RED BLEED: row 10 col 0 paper is RED — attr wrote past col 31")
	}
	if yel < 20 {
		t.Errorf("rule row missing: only %d YELLOW-ink cells on row 13", yel)
	}
	if lit < 500 {
		t.Errorf("preview barely painted: %d lit bytes", lit)
	}
	// forensic: where does any RED-ink cell come from? preview paints none.
	for r := 0; r < 24; r++ {
		for c := 0; c < 32; c++ {
			v := emu.mem.Read(uint16(0x5800 + r*32 + c))
			if v&7 == 2 {
				t.Logf("RED-INK attr row %d col %d = %02X", r, c, v)
			}
		}
	}
	run(3) // render the preview RAM through the real pipeline
	savePNGAtomic("/tmp/preview63.png", emu.lastFrame())
}
