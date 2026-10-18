package main

// balatroc_shopstack_test.go — SEVA report: "when changing order of
// jokers in store jokers are drawn on top of each other again".
// Shop keys (shop_screen switch): SEL=buy, DISC=reroll, PLAY=next,
// SORT=reorder-owned (pass 42, kmap SORT=(row1,0x02) 'S' after pass 46).
// Repro: boot +2, force jok[0]=3 jok[1]=5 (two owned jokers), jump into
// shop_screen ($A3EB), walk cursor onto owned row, stroke SORT, and
// census rows b17..b21 (y136..y176): stacked ghosts = ink columns that
// survive OUTSIDE the newly-painted card plates, or two texts sharing a
// column region. PNGs captured at every moment.

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
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var boardAddr uint16
	var boardData, musicData []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
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
	const shopScreen = 0xA47E
	const shopID = 0xFA53
	const shopSold = 0xFA63
	const shopCost = 0xFA55
	const shopCur = 0xFA65
	const bank = 0xF800
	const jok = 0xFA4A
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	for i, v := range musicData {
		emu.mem.Write(0x6000+uint16(i), v)
	}
	// two owned jokers BEFORE shop entry (shop_new won't touch jok[])
	emu.mem.Write(jok, 3)
	emu.mem.Write(jok+1, 5)
	emu.mem.Write(bank, 20)
	emu.mem.Write(0x7D00, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, 0x00)
	emu.mem.Write(0xFF57, 0x7D)
	emu.cpu.PC = shopScreen
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

	runF(90)
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
