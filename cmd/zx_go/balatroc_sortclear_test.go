package main

// balatroc_sortclear_test.go — pass-48d PROOF (Seva hardware bug: "jokers
// in store are still drawn on top of each other after moving a joker up
// using S key"). PASS-48g REBASE: reach the shop through the REAL win
// pipeline (balToShop), BUY two distinct jokers with real SEL strokes, put
// the cursor on the first owned slot, capture the interior pixels, press
// SORT twice (a full cycle homes the first occupant) and demand the slot
// interior is BYTE-IDENTICAL to the first capture. Any ghost of the
// swapped-out occupant = corrupted equality.
// Constants: board.map of tap c9827c58… (_shop_id=$FF14 _shop_cost=$FF16
// _shop_sold=$FF24 _shop_cur=$FF26 _bank=$FCB4 _jok=$FF0B).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocSortClearPlus2(t *testing.T) {
	if os.Getenv("SORTCLR") == "" {
		t.Skip("set SORTCLR=1")
	}
	const shopID = 0xFF14
	const shopSold = 0xFF24
	const shopCost = 0xFF16
	const shopCur = 0xFF26
	const bank = 0xFCB4
	const jok = 0xFF0B
	emu := balToShop(t, 3)
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
	buy := func(id byte) {
		emu.mem.Write(shopID, id)
		emu.mem.Write(shopSold, 0)
		emu.mem.Write(shopCost, 4)
		emu.mem.Write(bank, 20)
		emu.mem.Write(shopCur, 0)
		runF(20)
		stroke(4, 0x08, 60) // SEL -> shop_buy
	}
	buy(1)
	buy(2)
	runF(40)
	fmt.Printf("SORTCLR owned jok=%d,%d bank=%d PC=$%04X\n",
		emu.mem.Read(jok), emu.mem.Read(jok+1), emu.mem.Read(bank), emu.cpu.PC)
	if emu.mem.Read(jok) == 0 || emu.mem.Read(jok+1) == 0 {
		t.Fatalf("buys failed: jok=%d,%d", emu.mem.Read(jok), emu.mem.Read(jok+1))
	}
	// park cursor on the first owned slot, force a repaint via RIGHT+LEFT
	emu.mem.Write(shopCur, 4)
	stroke(2, 0x08, 20)
	stroke(2, 0x04, 20)
	// capture owned slot 0 interior: bx=1 -> bytes 2..5, rows 137..174
	cap0 := func() []byte {
		out := make([]byte, 0, 38*4)
		for y := 137; y <= 174; y++ {
			for b := 2; b <= 5; b++ {
				out = append(out, emu.mem.Read(uint16(0x4000+((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5)+b)))
			}
		}
		return out
	}
	first := cap0()
	lit := 0
	for _, v := range first {
		if v != 0 {
			lit++
		}
	}
	fmt.Printf("SORTCLR slot0 first paint lit=%d/152\n", lit)
	if lit == 0 {
		t.Fatal("owned slot 0 never painted — nothing to prove")
	}
	stroke(1, 0x02, 40) // SORT 1 — occupant cycles, joker 2 lands on slot 0
	mid := cap0()
	midlit := 0
	for _, v := range mid {
		if v != 0 {
			midlit++
		}
	}
	diff := 0
	for i := range first {
		if first[i] != mid[i] {
			diff++
		}
	}
	fmt.Printf("SORTCLR after sort1 lit=%d/152 changed-bytes=%d jok=%d,%d\n",
		midlit, diff, emu.mem.Read(jok), emu.mem.Read(jok+1))
	if diff == 0 {
		t.Fatal("SORT painted NOTHING new — slot pixels unchanged (sort did not repaint?)")
	}
	stroke(1, 0x02, 40) // SORT 2 — first joker home again
	home := cap0()
	d2 := 0
	for i := range first {
		if first[i] != home[i] {
			d2++
		}
	}
	if d2 != 0 {
		fmt.Printf("SORTCLR GHOST: %d bytes differ from clean first paint — stacked text\n", d2)
		t.Fatalf("sorted joker paints OVER previous text: %d dirty bytes in slot 0", d2)
	}
	fmt.Println("SORTCLR PASS: slot interior byte-identical after full cycle — no stacked jokers")
}
