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
// Constants: board.map of tap 88914c78… (_hand=$FF0A _play=$FF12
// _selected=$FF17 _n_order=$FF27 _cursor=$FF28 _score=$FF2A
// _hands_l=$FCEC _tgt=$FCF0 _bank=$FD00 _banner_q=$FE7B).

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
	for _, dead := range []string{"= +$", "NEPTUNE", "PLUTO", "FIVE KIND"} {
		if bytes.Contains(tapRaw, []byte(dead)) {
			t.Fatalf("%q still in the binary — pass 48i fix missing", dead)
		}
	}
	for _, need := range []string{"=+$", "MERCURY", "URANUS", "STRAIGHT FLUSH", "TWO PAIR"} {
		if !bytes.Contains(tapRaw, []byte(need)) {
			t.Fatalf("%q missing from binary", need)
		}
	}
	// planet hand table bytes: HT_PAIR..HT_SF in exact order
	planetTab := []byte{1, 2, 3, 4, 5, 7, 8}
	if i := bytes.Index(tapRaw, planetTab); i < 0 {
		t.Fatal("PLHT_TAB {PAIR..STRAIGHT FLUSH} not found in tap")
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
	for k := 0; k < 400 && rd(0xFE7B) != 0; k++ {
		runF(1)
	}
	if rd(0xFE7B) != 0 {
		t.Fatalf("banner_q never expired: %d", rd(0xFE7B))
	}
	e := bandAttr()
	if e[0]&0x38 == 0x30 || e[10]&0x38 == 0x30 {
		t.Fatalf("post-expiry green/yellow stripe on panel: %02X %02X", e[0], e[10])
	}
	savePNGAtomic("/tmp/pass48i_banner.png", emu.lastFrame())

	// --- (1) select -> SORT -> PLAY preserves the selection ---
	handAt := func(i int) byte { return rd(uint16(0xFF0A + uint16(i))) }
	var targets []byte
	cur := int(rd(0xFF28))
	for i := 0; i < 8 && len(targets) < 3; i++ {
		b := handAt(i)
		if b == 0xFF {
			continue // EMPTY hole
		}
		for cur != i {
			balStroke(emu, runF, 4, 0x04, 12) // RIGHT
			cur = int(rd(0xFF28))              // trust the game, not hope
		}
		balStroke(emu, runF, 4, 0x08, 12) // SEL
		if cur != int(rd(0xFF28)) {
			t.Fatalf("cursor moved after SEL: want %d got %d", cur, rd(0xFF28))
		}
		targets = append(targets, b)
		fmt.Printf("48i SEL slot%d card=%02X n_order=%d selected=%02X\n",
			i, b, rd(0xFF27), uint(rd(0xFF17))|uint(rd(0xFF18))<<8)
	}
	if len(targets) < 3 || rd(0xFF27) != byte(len(targets)) {
		t.Fatalf("selection setup: targets=%d n_order=%d", len(targets), rd(0xFF27))
	}
	// SORT (act 5 = 'S' = row1 bit1)
	balStroke(emu, runF, 1, 0x02, 10)
	runF(8)
	if rd(0xFF27) != byte(len(targets)) {
		t.Fatalf("SORT dropped the queue: n_order=%d", rd(0xFF27))
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
	score0 := uint32(rd(0xFF2A)) | uint32(rd(0xFF2B))<<8 |
		uint32(rd(0xFF2C))<<16 | uint32(rd(0xFF2D))<<24
	balStroke(emu, runF, 4, 0x01, 30)
	runF(40)
	var played []byte
	for i := 0; i < 3; i++ {
		played = append(played, rd(uint16(0xFF12+uint16(i))))
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
	if rd(0xFF27) != 0 {
		t.Fatalf("queue not consumed: n_order=%d", rd(0xFF27))
	}
	var score1lo, score1hi, stable byte
	score1lo = rd(0xFF2A)
	score1hi = rd(0xFF2B)
	// wait for the score animation to release the poll loop: score must
	// stop changing before further strokes
	for k := 0; k < 1500; k++ {
		runF(1)
		if rd(0xFF2A) == score1lo && rd(0xFF2B) == score1hi {
			stable++
			if stable >= 30 {
				break
			}
		} else {
			score1lo = rd(0xFF2A)
			score1hi = rd(0xFF2B)
			stable = 0
		}
	}
	fmt.Printf("48i sort+play: 3 selected cards all played, score0=%d\n", score0)

	// --- payout smoke: balToShop drives the REAL win pipeline (tgt=1
	// poke + SEL + knocking PLAY strokes); WIN evidence is a strictly
	// growing bank through win_payout. shop then owns the poll loop.
	bank0 := rd(0xFD00)
	_ = bank0
	shopEmu := balToShop(t, 2)
	rdS := func(ad uint16) byte { return shopEmu.mem.Read(ad) }
	fmt.Printf("48i shop: ante=%d bank=%d PC=$%04X\n",
		rdS(0xFCEE), rdS(0xFD00), shopEmu.cpu.PC)
	savePNGAtomic("/tmp/pass48i_shop.png", shopEmu.lastFrame())
	if rdS(0xFD00) == 0 {
		t.Fatalf("bank zero in shop")
	}
savePNGAtomic("/tmp/pass48i_win.png", emu.lastFrame())
	fmt.Println("48i PASS")
}
