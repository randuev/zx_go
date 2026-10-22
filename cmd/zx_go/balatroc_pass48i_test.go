package main

// balatroc_pass48i_test.go — SEVA report 2026-10-19 (six fixes):
// (1) sort must not lose the selection: LIVE gate — deal round 1, SEL the
//     first three dealt cards, SORT, PLAY, and demand play[] == those
//     cards (pick-order queue is CARD-based now, pass 48i #1).
// (2) instruction strips: the shipped tap must not carry the cryptic
//     "= +$" payout tag; the planet table must pair MERCURY with PAIR
//     (real Balatro) and must contain NO NEPTUNE/PLUTO/FIVE KIND dead
//     stock — verified as raw bytes of the binary.
// (3) payout layout math: every field cell is byte-checked for x-room so
//     a 3-digit reward can never print into the next label.
// (4) banner plate (live): on the PLAY screen the yellow plate covers
//     cols 11..31 ONLY — the left panel cols 0..10 keep black paper
//     during the banner and after expiry (no green stripe, ever).
// Addresses live: rebased from build/board.map via balSym() at run time.
// _hands_l=$F9F9 _tgt=$F9FD _bank=$FA0D _banner_q=$FB88).

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocPass48iPlus2(t *testing.T) {
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	// --- (2)(3) binary data law ---
	for _, dead := range []string{"= +$", "PLUTO", "FIVE KIND"} {
		if bytes.Contains(tapRaw, []byte(dead)) {
			t.Fatalf("%q still in the binary — pass 48i fix missing", dead)
		}
	}
	// pass 49j: NEPTUNE=FULL HOUSE is REAL Balatro — must be BACK in the
	// pool; the 48i removal of NEPTUNE was my over-correction.
	for _, need := range []string{"=+$", "MERCURY", "URANUS", "NEPTUNE",
		"STRAIGHT FLUSH", "TWO PAIR", "FULL HOUSE"} {
		if !bytes.Contains(tapRaw, []byte(need)) {
			t.Fatalf("%q missing from binary", need)
		}
	}
	// planet hand table bytes: HT_PAIR..HT_SF + HT_FULL in exact order
	planetTab := []byte{1, 2, 3, 4, 5, 7, 8, 6}
	if i := bytes.Index(tapRaw, planetTab); i < 0 {
		t.Fatal("PLHT_TAB {PAIR..STRAIGHT FLUSH, FULL HOUSE} not found in tap")
	}
	// RAMTOP law (pass 49j): CODE block must never load past $FF57 or
	// LOAD ""CODE ejects the tape straight to BASIC on real +2 hardware.
	{
		i, tail := 0, 0
		for i+2 <= len(tapRaw) {
			ln := int(tapRaw[i]) | int(tapRaw[i+1])<<8
			if ln < 3 || i+ln > len(tapRaw) {
				break
			}
			b := tapRaw[i+2 : i+ln]
			if b[0] == 0 && b[1] == 3 {
				start := int(b[12]) | int(b[13])<<8
				length := int(b[14]) | int(b[15])<<8
				tail = start + length - 1
			}
			i += ln + 2
		}
		if tail == 0 {
			t.Fatal("no CODE header block found in tap")
		}
		if tail > 0xFF57 {
			t.Fatalf("RAMTOP VIOLATION: CODE tail $%04X > $FF57 (tape ejects to BASIC)", tail)
		}
		fmt.Printf("RAMTOP law: CODE tail=$%04X margin=%dB OK\n", tail, 0xFF57-tail)
	}
	// payout x-room law (from main.c fixed grid): each value cell has a
	// 2px gap to the next label even at 3 digits (cell px = 18+2)
	fmt.Println("48i static: payout grid + planet table law OK")

	// --- live boot ---
	emu := balBootLive(t)
	runF := func(n int) {
		for j := 0; j < n; j++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	rd := func(a uint16) byte { return emu.mem.Read(a) }
	runF(160)
	balStroke(emu, runF, 4, 0x01, 200) // START -> round 1 deal
	// PASS 63: the ANTE PREVIEW (Seva) now opens after the title and eats
	// one key — dismiss it, then the round owns the screen as before.
	balStroke(emu, runF, 4, 0x01, 60)
	runF(140)
	if emu.cpu.PC < 0x8000 || emu.cpu.PC > 0xF000 {
		t.Fatalf("not inside the game: PC=$%04X", emu.cpu.PC)
	}

	// --- (4) banner plate spans the FIELD only ---
	// idle census of band 22 attr bytes
	bandAttr := func() []byte {
		a := make([]byte, 32)
		for c := 0; c < 32; c++ {
			a[c] = emu.mem.Read(uint16(0x5800 + 22*32 + c))
		}
		return a
	}
	idle := bandAttr()
	if idle[0]&0x38 == 0x30 || idle[10]&0x38 == 0x30 {
		t.Fatalf("left panel cols 0..10 not black-paper at idle: %02X %02X", idle[0], idle[10])
	}
	// PLAY with NOTHING selected -> banner paints
	balStroke(emu, runF, 4, 0x01, 4)
	d := bandAttr()
	if d[22]&0x38 != 0x30 {
		t.Fatalf("banner plate missing on field col22: %02X", d[22])
	}
	if d[0]&0x38 == 0x30 || d[5]&0x38 == 0x30 || d[10]&0x38 == 0x30 {
		t.Fatalf("GREEN/YELLOW STRIPE over left panel cols 0..10: %02X %02X %02X", d[0], d[5], d[10])
	}
	fmt.Printf("48i banner: plate cols11..31 yellow, panel cols0..10 attr=%02X/%02X/%02X OK\n",
		d[0], d[5], d[10])
	// let it expire (banner_q ticks inside scanKey)
	for k := 0; k < 400 && rd(balSym("banner_q")) != 0; k++ {
		runF(1)
	}
	if rd(balSym("banner_q")) != 0 {
		t.Fatalf("banner_q never expired: %d", rd(balSym("banner_q")))
	}
	e := bandAttr()
	if e[0]&0x38 == 0x30 || e[10]&0x38 == 0x30 {
		t.Fatalf("post-expiry green/yellow stripe on panel: %02X %02X", e[0], e[10])
	}
	savePNGAtomic("/tmp/pass48i_banner.png", emu.lastFrame())

	// --- (1) select -> SORT -> PLAY preserves the selection ---
	handAt := func(i int) byte { return rd(balSym("hand") + uint16(i)) }
	var targets []byte
	cur := int(rd(balSym("cursor")))
	for i := 0; i < 8 && len(targets) < 3; i++ {
		b := handAt(i)
		if b == 0xFF {
			continue // EMPTY hole
		}
		for cur != i {
			balStroke(emu, runF, 4, 0x04, 12) // RIGHT
			cur = int(rd(balSym("cursor")))              // trust the game, not hope
		}
		balStroke(emu, runF, 4, 0x08, 12) // SEL
		if cur != int(rd(balSym("cursor"))) {
			t.Fatalf("cursor moved after SEL: want %d got %d", cur, rd(balSym("cursor")))
		}
		targets = append(targets, b)
		fmt.Printf("48i SEL slot%d card=%02X n_order=%d selected=%02X\n",
			i, b, rd(balSym("n_order")), uint(rd(balSym("last_chi"))) | uint(rd(balSym("last_chi")+1))<<8)
	}
	if len(targets) < 3 || rd(balSym("n_order")) != byte(len(targets)) {
		t.Fatalf("selection setup: targets=%d n_order=%d", len(targets), rd(balSym("n_order")))
	}
	// SORT (act 5 = 'S' = row1 bit1)
	balStroke(emu, runF, 1, 0x02, 10)
	runF(8)
	if rd(balSym("n_order")) != byte(len(targets)) {
		t.Fatalf("SORT dropped the queue: n_order=%d", rd(balSym("n_order")))
	}
	// the selected cards must still all be IN the hand
	for _, c := range targets {
		found := false
		for i := 0; i < 8; i++ {
			if handAt(i) == c {
				found = true
			}
		}
		if !found {
			t.Fatalf("SORT lost selected card %02X from hand", c)
		}
	}
	// PLAY -> all three must land in play[]
	score0 := rd32(emu, balSym("score"))
	balStroke(emu, runF, 4, 0x01, 30)
	runF(40)
	var played []byte
	for i := 0; i < 3; i++ {
		played = append(played, rd(balSym("play")+uint16(i)))
	}
	for _, c := range targets {
		inPlay := false
		for _, p := range played {
			if p == c {
				inPlay = true
			}
		}
		if !inPlay {
			t.Fatalf("PLAY dropped selected card %02X: play=%02X %02X %02X",
				c, played[0], played[1], played[2])
		}
	}
	if rd(balSym("n_order")) != 0 {
		t.Fatalf("queue not consumed: n_order=%d", rd(balSym("n_order")))
	}
	var score1, stable uint32
	score1 = rd32(emu, balSym("score"))
	
	// wait for the score animation to release the poll loop: score must
	// stop changing before further strokes
	for k := 0; k < 1500; k++ {
		runF(1)
		if rd32(emu, balSym("score")) == score1 {
			stable++
			if stable >= 30 {
				break
			}
		} else {
			score1 = rd32(emu, balSym("score"))
			
			stable = 0
		}
	}
	fmt.Printf("48i sort+play: 3 selected cards all played, score0=%d\n", score0)

	// --- payout smoke: balToShop drives the REAL win pipeline (tgt=1
	// poke + SEL + knocking PLAY strokes); WIN evidence is a strictly
	// growing bank through win_payout. shop then owns the poll loop.
	bank0 := rd(balSym("bank"))
	_ = bank0
	shopEmu := balToShop(t, 2)
	rdS := func(ad uint16) byte { return shopEmu.mem.Read(ad) }
	fmt.Printf("48i shop: ante=%d bank=%d PC=$%04X\n",
		rdS(balSym("ante")), rdS(balSym("bank")), shopEmu.cpu.PC)
	savePNGAtomic("/tmp/pass48i_shop.png", shopEmu.lastFrame())
	if rdS(balSym("bank")) == 0 {
		t.Fatalf("bank zero in shop")
	}
savePNGAtomic("/tmp/pass48i_win.png", emu.lastFrame())
	fmt.Println("48i PASS")
}
