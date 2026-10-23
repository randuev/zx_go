package main

// balatroc_tarot_test.go — PASS 64 TAROT LAW GATE (TAROT=1).
//
// The TAROT law, proven through the game's REAL input paths:
//   SHOP  stock chip rolls per visit (tar_new at shop_new);
//         'T' buys for exactly $3 (tar_buy: bank-3, tray fills, stock
//         sold); tray FULL -> refuse, stock NOT consumed; short of $ ->
//         refuse, nothing consumed; DISC rerolls stock (tar_new).
//   ROUND 'T' arms the picker (tar_pick owns its own scanKey loop);
//         PLAY with WRONG mark count refuses (tray stays); SEL marks,
//         apply at EXACT count stamps the badge / graves the card;
//         tray slot consumed on apply; HANGED MAN graves: alive=0 and
//         ndeck recounted EXACTLY (GRAVE law — ndeck is deck truth).
//
// Honest-path law: arrive at the shop through the REAL win pipeline
// (balToShop), exit via NEXT=PLAY, drive every step with strokes.
// Pokes (bank/tar_id/tray/hand/tgt) are test scaffolding; purchase,
// picker, apply and recount are 100% game code.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocTarotPlus2(t *testing.T) {
	if os.Getenv("TAROT") == "" {
		t.Skip("set TAROT=1")
	}
	emu := balToShop(t, 2)
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	stroke := func(row byte, bit byte, tail int) { balStroke(emu, runF, row, bit, tail) }
	rd := func(a uint16) byte { return emu.mem.Read(a) }
	w := func(a uint16, v byte) { emu.mem.Write(a, v) }
	w16 := func(a uint16, v uint16) { w(a, byte(v)); w(a+1, byte(v>>8)) }
	rd16 := func(a uint16) uint16 { return uint16(rd(a)) | uint16(rd(a+1))<<8 }
	w32 := func(a uint16, v uint32) {
		for j := 0; j < 4; j++ {
			w(a+uint16(j), byte(v>>uint(8*j)))
		}
	}

	tarID := balSym("tar_id")
	tray := balSym("tray")
	bank := balSym("bank")
	alive := balSym("alive")
	ndeck := balSym("ndeck")
	enh := balSym("enh")
	hand := balSym("hand")
	sel := balSym("selected")
	tgt := balSym("tgt")
	handsL := balSym("hands_l")
	for _, p := range []struct {
		n string
		a uint16
	}{{"tar_id", tarID}, {"tray", tray}, {"bank", bank}, {"alive", alive},
		{"ndeck", ndeck}, {"enh", enh}, {"hand", hand}, {"selected", sel},
		{"tgt", tgt}, {"hands_l", handsL}} {
		if p.a == 0 {
			t.Fatalf("missing symbol %s", p.n)
		}
	}
	// knock a stroke until a condition holds (busy loops drop strokes)
	knock := func(row byte, bit byte, cond func() bool, max int) bool {
		for i := 0; i < max; i++ {
			stroke(row, bit, 2)
			runF(30)
			if cond() {
				return true
			}
		}
		return false
	}
	// kmap law: '6' lives on the digit row (row 4 bit 0x10)
	K_T, K_SEL, K_PLAY, K_DISC := byte(2), byte(4), byte(4), byte(4)
	B_T, B_SEL, B_PLAY, B_DISC := byte(0x10), byte(0x08), byte(0x01), byte(0x10)

	// ===== SHOP LAW =====
	tid := rd(tarID)
	if tid < 1 || tid > 10 {
		t.Fatalf("stock chip not rolled on shop entry: tar_id=%d PC=$%04X", tid, emu.cpu.PC)
	}
	if rd(tray) != 0 || rd(tray+1) != 0 {
		t.Fatalf("tray not empty at shop entry: %d,%d", rd(tray), rd(tray+1))
	}
	fmt.Printf("shop live: stock tar_id=%d tray=%d,%d bank=%d\n", tid, rd(tray), rd(tray+1), rd16(bank))

	// BUY for exactly $3
	w16(bank, 99)
	got := knock(K_T, B_T, func() bool { return rd(tray) != 0 }, 8)
	if !got {
		t.Fatalf("'T' never bought: tar_id=%d tray=%d,%d bank=%d PC=$%04X",
			rd(tarID), rd(tray), rd(tray+1), rd16(bank), emu.cpu.PC)
	}
	if b := rd16(bank); b != 96 {
		t.Fatalf("buy charged wrong price: bank=%d want 96", b)
	}
	if rd(tarID) != 0 {
		t.Fatalf("stock not sold after buy: tar_id=%d", rd(tarID))
	}
	fmt.Printf("BUY law: tray[0]=%d bank 99->%d stock sold OK\n", rd(tray), rd16(bank))

	// fill tray, then FULL refusal: stock survives, bank frozen
	w(tarID, 8)
	runF(20)
	if !knock(K_T, B_T, func() bool { return rd(tray+1) != 0 }, 8) {
		t.Fatalf("second buy failed: tray=%d,%d bank=%d", rd(tray), rd(tray+1), rd16(bank))
	}
	w(tarID, 9)
	frozenBank := rd16(bank)
	for i := 0; i < 4; i++ {
		stroke(K_T, B_T, 30)
	}
	if rd(tray) != 8 || rd(tray+1) != 8 || rd(tarID) != 9 || rd16(bank) != frozenBank {
		t.Fatalf("tray-FULL law broken: tray=%d,%d tar_id=%d bank=%d frozen=%d",
			rd(tray), rd(tray+1), rd(tarID), rd16(bank), frozenBank)
	}
	fmt.Printf("FULL law: tray stuck at %d,%d, stock %d intact, bank frozen OK\n",
		rd(tray), rd(tray+1), rd(tarID))
	// tray[1]=8 because the second buy was the poked stock 8 — the FIRST
	// bought id sits in tray[0] (the roll); assert law shape, not the roll.
	if !(rd(tray) >= 1 && rd(tray) <= 10) || rd(tray+1) != 8 {
		t.Fatalf("tray law shape broken: %d,%d", rd(tray), rd(tray+1))
	}

	// short-of-$ refusal: EMPTY tray this time so the FULL branch can't
	// shadow it (tar_buy checks tray-full BEFORE the bank); nothing consumed
	w(tray, 0)
	w(tray+1, 0)
	w(tarID, 9)
	w16(bank, 2)
	for i := 0; i < 3; i++ {
		stroke(K_T, B_T, 30)
	}
	if rd16(bank) != 2 || rd(tarID) != 9 || rd(tray) != 0 || rd(tray+1) != 0 {
		t.Fatalf("broke law: bank=%d tar_id=%d tray=%d,%d", rd16(bank), rd(tarID), rd(tray), rd(tray+1))
	}
	fmt.Printf("SHORT law: bank=2 refusal, stock %d intact OK\n", rd(tarID))

	// DISC rerolls stock (and costs the reroll fee); a tray holding a
	// tarot must ride the reroll untouched
	w(tray, 8)
	w(tray+1, 0)
	w16(bank, 99)
	w(tarID, 9)
	runF(20)
	trayA, trayB := rd(tray), rd(tray+1)
	rb := rd16(bank)
	rolled := knock(K_DISC, B_DISC, func() bool { return rd(tarID) != 0 && rd16(bank) < rb }, 6)
	if !rolled {
		t.Fatalf("DISC never rerolled: tar_id=%d bank=%d PC=$%04X", rd(tarID), rd16(bank), emu.cpu.PC)
	}
	if rd(tray) != trayA || rd(tray+1) != trayB {
		t.Fatalf("reroll touched the tray: %d,%d want %d,%d", rd(tray), rd(tray+1), trayA, trayB)
	}
	fmt.Printf("REROLL law: stock %d->%d fee paid, tray intact OK\n", 9, rd(tarID))

	// ===== ROUND PICKER LAW =====
	// The shop's only door is NEXT=PLAY, but busy loops eat strokes — knock
	// until round_no ADVANCES (the law speaking for itself), then let the
	// deal finish (sort_hand reorders slots after the deal) before parking
	// the test card. Identity law: enh rides the CARD, not the slot — we
	// track the parked card's LIVE slot and step the picker cursor onto it.
	w(tray, 5)
	w(tray+1, 0)
	w16(bank, 50)
	roundNo := balSym("round_no")
	r0 := rd(roundNo)
	if !knock(K_PLAY, B_PLAY, func() bool { return rd(roundNo) != r0 }, 12) {
		t.Fatalf("shop NEXT never ran: round_no=%d PC=$%04X", rd(roundNo), emu.cpu.PC)
	}
	runF(220) // deal + sort_hand settle
	w32(tgt, 999999)
	// pick a test card NOT already dealt (card identity = unique truth)
	var victim byte = 0xFF
search:
	for c := 0; c < 52; c++ {
		inHand := false
		for j := 0; j < 8; j++ {
			if rd(hand+uint16(j)) == byte(c) {
				inHand = true
			}
		}
		if !inHand && rd(alive+uint16(c)) != 0 && rd(enh+uint16(c)) == 0 {
			victim = byte(c)
			break search
		}
	}
	if victim == 0xFF {
		t.Fatalf("no free card to park")
	}
	parked := false
	for i := 0; i < 60; i++ {
		// settled deal = all 8 slots non-hole AND stable for 120 frames;
		// then overwrite slot0 with the victim (identity law)
		full := true
		for j := 0; j < 8; j++ {
			if rd(hand+uint16(j)) >= 52 { // EMPTY = 0xFF is the only hole
				full = false
			}
		}
		if !full {
			runF(20)
			continue
		}
		stable := true
		var snap [8]byte
		for j := 0; j < 8; j++ {
			snap[j] = rd(hand + uint16(j))
		}
		runF(120)
		for j := 0; j < 8; j++ {
			if rd(hand+uint16(j)) != snap[j] {
				stable = false
			}
		}
		if !stable {
			continue
		}
		w(hand, victim)
		w(sel, 0)
		runF(60)
		if rd(hand) == victim {
			parked = true
			break
		}
	}
	if !parked {
		t.Fatalf("card %d never parks: hand[0]=%d PC=$%04X", victim, rd(hand), emu.cpu.PC)
	}

	// live slot of the parked card + picker cursor start = first non-hole
	stepTo := func(card byte) {
		first, at := -1, -1
		for j := 0; j < 8; j++ {
			c := rd(hand + uint16(j))
			if c < 52 { // EMPTY is 0xFF only — card 0 (2C) is a real card!
				if first < 0 {
					first = j
				}
				if c == card && at < 0 {
					at = j
				}
			}
		}
		if at < 0 || first < 0 {
			t.Fatalf("card %d vanished from hand before stepping", card)
		}
		for s := at - first; s > 0; s-- {
			stroke(4, 0x04, 8) // RIGHT
		}
	}

	// arm picker, PLAY at ZERO marks -> refuse, tray stays
	stepTo(victim)
	stroke(K_PLAY, B_PLAY, 40)
	if rd(tray) != 5 {
		t.Fatalf("picker PLAY at 0 marks consumed tray (no EXACT-count law): tray=%d,%d PC=$%04X",
			rd(tray), rd(tray+1), emu.cpu.PC)
	}
	if rd(enh+uint16(victim)) != 0 {
		t.Fatalf("refusal stamped anyway: enh[%d]=%d", victim, rd(enh+uint16(victim)))
	}
	fmt.Printf("WRONG-count law: PLAY@0 marks refused, tray intact OK\n")

	// SEL into aim-mode and mark the parked card, then PLAY — knocked as
	// SEL+PLAY pairs so the apply converges regardless of latch parity
	// (a held SEL can double-fire through scanKey and toggle the mark
	// off again); PLAY@0-marks harmlessly refuses, PLAY@1 applies.
	// SEL into aim-mode, mark, PLAY — knocked as full triples. The picker
	// EXITS on every mode-2 PLAY (applied or refused), and latch parity
	// can toggle the mark off — so each knock re-arms mode2, re-steps and
	// marks before PLAY; a refused (0-mark) PLAY costs nothing but re-aims.
	applied := false
	for k := 0; k < 6 && !applied; k++ {
		stroke(K_SEL, B_SEL, 40) // mode 1 -> mode 2
		stepTo(victim)
		stroke(K_SEL, B_SEL, 40) // mark the parked slot
		stroke(K_PLAY, B_PLAY, 80)
		applied = rd(enh+uint16(victim)) == 5 // ENH_GOLD
		if !applied && rd(tray) != 0 {
			// picker exited on a 0-mark refusal — tray survives, re-arm
			stroke(K_T, B_T, 40)
		}
	}
	if !applied {
		var hb [8]byte
		var eb []int
		for j := 0; j < 8; j++ {
			hb[j] = rd(hand + uint16(j))
		}
		for j := 0; j < 52; j++ {
			if rd(enh+uint16(j)) != 0 {
				eb = append(eb, j*100+int(rd(enh+uint16(j))))
			}
		}
		t.Fatalf("EXACT-count apply: enh[%d]=%d want 5 (GOLD) PC=$%04X handsL=%d tray=%d,%d hand=%v enh!=0=%v",
			victim, rd(enh+uint16(victim)), emu.cpu.PC, rd(handsL), rd(tray), rd(tray+1), hb, eb)
	}
	if rd(tray) != 0 || rd(tray+1) != 0 {
		t.Fatalf("apply did not consume the tray: %d,%d", rd(tray), rd(tray+1))
	}
	fmt.Printf("APPLY law: tid5 -> enh[%d]=GOLD, tray consumed OK\n", victim)

	// HANGED MAN: grave + hole + exact ndeck recount — victim is the SAME
	// card (deterministic: same identity, fresh picker entry)
	w(tray, 10)
	w(tray+1, 0)
	if rd(alive+uint16(victim)) == 0 {
		t.Fatalf("card %d not alive before hanging: alive=%d", victim, rd(alive+uint16(victim)))
	}
	nd0 := rd(ndeck)
	stroke(K_T, B_T, 40) // arm (tid 10)
	hanged := false
	for k := 0; k < 6 && !hanged; k++ {
		stroke(K_SEL, B_SEL, 40) // mode 1 -> mode 2
		stepTo(victim)
		stroke(K_SEL, B_SEL, 40) // mark the slot holding the victim
		stroke(K_PLAY, B_PLAY, 80)
		hanged = rd(alive+uint16(victim)) == 0
		if !hanged && rd(tray) != 0 {
			stroke(K_T, B_T, 40) // re-arm after a refusal exit
		}
	}
	runF(60)
	if rd(alive+uint16(victim)) != 0 {
		t.Fatalf("HANGED MAN: alive[%d] still %d PC=$%04X", victim, rd(alive+uint16(victim)), emu.cpu.PC)
	}
	if nd1 := rd(ndeck); nd1 != nd0-1 {
		t.Fatalf("GRAVE recount: ndeck %d->%d want %d", nd0, nd1, nd0-1)
	}
	if rd(tray) != 0 || rd(tray+1) != 0 {
		t.Fatalf("HANGED MAN tray not consumed: %d,%d", rd(tray), rd(tray+1))
	}
	if rd(hand) != 0xFF {
		t.Fatalf("HANGED MAN left no hole: hand[0]=%d", rd(hand))
	}
	fmt.Printf("HANGED MAN law: card %d graved, ndeck %d->%d exact OK\n", victim, nd0, rd(ndeck))

	fmt.Println("PASS TAROT: shop buy/full/short/reroll + round picker wrong-count/exact/gravest — all exact")
}
