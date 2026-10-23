package main

// balatroc_decklaw_test.go — PASS 65 DECK FACE law (DECKLAW=1).
// After suit tarots ref-face cards in place, the STATS grid counts
// FACES, not ids: two live cards sharing a face stamp ONE cell with a
// live-count badge, and a suit missing a card keeps a plain empty cell.
// Law proven with real page paint: poke a known deck truth (identity
// deck + one ref-face duplicate), invoke the PAGE paint_stats through its
// framed $C000 door (bank 0 window), then census screen RAM:
//   - club row, rank-2 cell: rank glyph + badge digit '2' both lit
//   - heart row, rank-2 cell: completely dark (that face no longer exists)
//   - ndeck/alive: no card silently lost (52 alive, both ids lit)

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocDeckLawPlus2(t *testing.T) {
	if os.Getenv("DECKLAW") == "" {
		t.Skip("set DECKLAW=1")
	}
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Skipf("no tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var core []byte
	var coreAddr uint16
	pages := map[int][]byte{}
	for i := range blocks {
		b := blocks[i]
		if !b.hdr || b.typ != 3 || i+1 >= len(blocks) {
			continue
		}
		switch trimName(b.name) {
		case "core":
			coreAddr, core = b.start, blocks[i+1].data
		case "page_game":
			pages[0] = blocks[i+1].data
		case "page_shop":
			pages[1] = blocks[i+1].data
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
	emu.paused.Store(false)
	for i, v := range core {
		emu.mem.Write(coreAddr+uint16(i), v)
	}
	for p, d := range pages {
		balBankSel(t, emu, byte(p))
		for i, v := range d {
			emu.mem.Write(0xC000+uint16(i), v)
		}
	}
	balBankSel(t, emu, 2)

	rd := func(a uint16) byte { return emu.mem.Read(a) }
	w := func(a uint16, v byte) { emu.mem.Write(a, v) }
	val := balSym("val")
	alive := balSym("alive")
	deck := balSym("deck")
	top := balSym("top")
	ndeck := balSym("ndeck")

	// identity deck, everything alive
	for k := 0; k < 52; k++ {
		w(val+uint16(k), byte(k))
		w(alive+uint16(k), 1)
		w(deck+uint16(k), byte(k))
	}
	w(top, 0)
	w(ndeck, 52)
	// ref-face: 2♥ (face 2*13+0=26, identity id 26) becomes a 2♣
	// duplicate — in place (PASS 65 tarot law).
	w(val+26, 0)

	balBankSel(t, emu, 0)
	ps := balPageSym("paint_stats")
	balCallNoArg(t, emu, ps)

	pa := func(y, bx int) byte {
		return emu.mem.Read(uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx))
	}
	lit := func(y0, y1, bx0, bx1 int) int {
		n := 0
		for y := y0; y <= y1; y++ {
			for bx := bx0; bx <= bx1; bx++ {
				if pa(y, bx) != 0 {
					n++
				}
			}
		}
		return n
	}
	// grid rows top->bottom S,H,D,C with sv=3-s:
	//   s=0 y0=120 sv=3 (S) | s=1 y0=136 sv=2 (H)
	//   s=2 y0=152 sv=1 (D) | s=3 y0=168 sv=0 (C)
	// rank-2 cell cx=16 -> byte cols 2..3. text6 rank glyph rides rows
	// y0..y0+7; the duplicate badge rides text row0+1 -> y0+8..y0+15,
	// byte col 3 (cx+10=26 -> 26/8=3).
	clubCell := lit(168, 175, 2, 3)
	clubBadge := lit(176, 183, 3, 3)
	// heart row s=1 y0=136: rank-2 cell must be dark. Cell content
	// (rank 6|12px at cx+2/+5, badge at cx+10) lives in byte cols 2..3;
	// bx4 already belongs to the k=1 neighbour cell.
	hDark := lit(136, 143, 2, 3)
	fmt.Printf("DECKLAW clubCell=%d badge=%d heartLit=%d ndeck=%d alive0=%d alive26=%d\n",
		clubCell, clubBadge, hDark, rd(ndeck), rd(alive), rd(alive+26))
	if clubCell == 0 {
		t.Errorf("club rank-2 cell dark — duplicate face not stamped")
	}
	if clubBadge == 0 {
		t.Errorf("duplicate badge missing — face count >1 must show a digit")
	}
	if hDark != 0 {
		t.Errorf("heart rank-2 cell lit (%d bytes) — refaced card still counted at its OLD face", hDark)
	}
	if rd(ndeck) != 52 || rd(alive) != 1 || rd(alive+26) != 1 {
		t.Errorf("cards lost: ndeck=%d alive0=%d alive26=%d", rd(ndeck), rd(alive), rd(alive+26))
	}
	fmt.Println("PASS DECKLAW: faces counted, duplicates badged, refaced moved, nothing lost")
}
