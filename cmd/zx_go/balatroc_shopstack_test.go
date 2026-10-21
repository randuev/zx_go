package main

// balatroc_shopstack_test.go — SEVA report: "when changing order of
// jokers in store jokers are drawn on top of each other again".
// PASS-48g REBASE: arrive at the shop through the REAL win pipeline
// (balToShop), BUY two jokers with real SEL strokes (engine-owned, so
// shop_new cannot wipe the fake jok state the old harness poked), then
// SORT and census rows b17..b21 (y136..y176): stacked ghosts = ink OUTSIDE
// the plate span (col 0 or col >12 for two cards), or jok[] ids lost.
// Addresses live: rebased from build/board.map via balSym() at run time.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocShopStackPlus2(t *testing.T) {
	if os.Getenv("SHOPSTACK") == "" {
		t.Skip("set SHOPSTACK=1")
	}
	shopID := balSym("shop_id")
	shopSold := balSym("shop_sold")
	shopCost := balSym("shop_cost")
	shopCur := balSym("shop_cur")
	bank := balSym("bank")
	jok := balSym("jok")
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
	ownedInk := func(tag string) [32]int {
		var cols [32]int
		for bx := 0; bx < 32; bx++ {
			for y := 136; y <= 176; y++ {
				if emu.mem.Read(uint16(0x4000+(((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5))+bx)) != 0 {
					cols[bx]++
				}
			}
		}
		fmt.Printf("%s jok=%d,%d cols:", tag, emu.mem.Read(jok), emu.mem.Read(jok+1))
		for bx := 0; bx < 32; bx++ {
			if cols[bx] > 0 {
				fmt.Printf(" %02X(%d)", bx, cols[bx])
			}
		}
		fmt.Println()
		return cols
	}
	// BUY two jokers for real: force cheap, never-sold ids and stroke SEL
	// twice; between buys make sure bank covers the cost again.
	buy := func(id byte) {
		emu.mem.Write(shopID, id)
		emu.mem.Write(shopSold, 0)
		emu.mem.Write(shopCost, 4)
		emu.mem.Write(bank, 20)
		emu.mem.Write(shopCur, 0)
		runF(20) // let shop repaint reflect the forced offer
		stroke(4, 0x08, 60) // SEL -> shop_buy
	}
	buy(3)
	buy(5)
	runF(60)
	fmt.Printf("after buys: jok=%d,%d bank=%d\n",
		emu.mem.Read(jok), emu.mem.Read(jok+1), emu.mem.Read(bank))
	if emu.mem.Read(jok) == 0 && emu.mem.Read(jok+1) == 0 {
		t.Fatalf("buys landed nowhere: jok=%d,%d PC=$%04X",
			emu.mem.Read(jok), emu.mem.Read(jok+1), emu.cpu.PC)
	}
	ownedInk("entry")
	savePNGAtomic("/tmp/shop1_entry.png", emu.lastFrame())
	// walk cursor onto owned joker 0: RIGHT x4 (offer0->1->pl0->pl1->own0)
	for i := 0; i < 4; i++ {
		stroke(4, 0x04, 20)
	}
	fmt.Printf("shop_cur=%d\n", emu.mem.Read(shopCur))
	savePNGAtomic("/tmp/shop2_own0.png", emu.lastFrame())
	// SORT: cycle joker row — the reorder that "draws jokers on top"
	stroke(1, 0x02, 60)
	fmt.Printf("after SORT jok=%d,%d shop_cur=%d\n",
		emu.mem.Read(jok), emu.mem.Read(jok+1), emu.mem.Read(shopCur))
	c2 := ownedInk("sort1")
	savePNGAtomic("/tmp/shop3_after_sort.png", emu.lastFrame())
	// attr census b17..b21: every byte, so the swipe-uncovered cells are
	// visible as stale attribute bytes on the swapped side
	fmt.Print("attr b17-b21 rows:")
	for ry := 17; ry <= 21; ry++ {
		fmt.Printf("\n r%02d:", ry)
		for bx := 0; bx < 32; bx++ {
			if bx >= 1 && bx <= 12 {
				v := emu.mem.Read(uint16(0x5800 + ry*32 + bx))
				fmt.Printf(" %02X", v)
			} else {
				fmt.Print("   ")
			}
		}
	}
	fmt.Println()
	// LAW: owned row ink must live ONLY inside the plates (cols 1..12
	// for two cards) — anything at col 0 or col >=13 is a drawn-on-top
	// orphan of a moved card. Plate 1's own border sits at col 7.
	outside := 0
	for bx := range c2 {
		if (bx < 1 || bx > 12) && c2[bx] > 0 {
			outside += c2[bx]
		}
	}
	fmt.Printf("ink-outside-plates=%d\n", outside)
	if outside > 0 {
		t.Fatalf("ghost ink outside plate span after SORT: %d", outside)
	}
	// re-verify targets moved: swap must be reflected in jok[]
	if !(emu.mem.Read(jok) == 3 && emu.mem.Read(jok+1) == 5) &&
		!(emu.mem.Read(jok) == 5 && emu.mem.Read(jok+1) == 3) {
		t.Fatalf("SORT clobbered jok ids: %d,%d", emu.mem.Read(jok), emu.mem.Read(jok+1))
	}
}
