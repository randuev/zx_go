package main

// balatroc_f10_test.go — SEVA report 2026-10-18: "F+10 does not render
// when bought". PASS-48g REBASE: reach the shop through the REAL win
// pipeline (balToShop), force the left offer to id 11 (F+10) with real
// shop state, stroke SEL (the buy key), and census the owned-joker card
// pixels + jok[] state. Then EXIT the shop (PLAY = NEXT) and census the
// BOARD joker cell — the joker must render on the table too.
// Constants: board.map of tap c9827c58… (_shop_id=$0xFC6F _shop_cost=$0xFC71
// _shop_sold=$0xFC7F _shop_cur=$0xFC81 _bank=$0xFA0D _jok=$0xFC66).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocF10ShopPlus2(t *testing.T) {
	if os.Getenv("F10") == "" {
		t.Skip("set F10=1")
	}
	const shopID = 0xFC6F
	const shopSold = 0xFC7F
	const shopCost = 0xFC71
	const shopCur = 0xFC81
	const bank = 0xFA0D
	const jok = 0xFC66
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
	inkWin := func(y0, y1 int, bx0, bx1 int) int {
		n := 0
		for y := y0; y <= y1; y++ {
			for bx := bx0; bx <= bx1; bx++ {
				if emu.mem.Read(uint16(0x4000+(((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5))+bx)) != 0 {
					n++
				}
			}
		}
		return n
	}
	// FORCE left offer = F+10 (id 11), affordable, cursor on it
	emu.mem.Write(shopID, 11)
	emu.mem.Write(shopSold, 0)
	emu.mem.Write(shopCost, 4)
	emu.mem.Write(shopID+1, 4) // right offer away from 11 dupes
	emu.mem.Write(bank, 20)
	emu.mem.Write(shopCur, 0)
	runF(20) // repaint shows the forced offer
	inkBefore := inkWin(136, 175, 1, 6)
	stroke(4, 0x08, 80) // SEL -> shop_buy(11)
	j0 := emu.mem.Read(jok)
	inkAfter := inkWin(136, 175, 1, 6)
	fmt.Printf("after buy: jok[0]=%d bank=%d ink owned-slot0 before=%d after=%d PC=$%04X\n",
		j0, emu.mem.Read(bank), inkBefore, inkAfter, emu.cpu.PC)
	if j0 != 11 {
		t.Fatalf("BUY failed: jok[0]=%d want 11 (PC=$%04X)", j0, emu.cpu.PC)
	}
	if inkAfter == 0 {
		t.Fatalf("owned F+10 card renders NOTHING in slot 0")
	}
	savePNGAtomic("/tmp/f10_shop.png", emu.lastFrame())
	// NEXT leaves the shop -> next_round -> draw_board. Census the BOARD
	// joker cell (cols 12..15, rows 8..47) — the bought joker must render
	// on the table too (SEVA's original report: "does not render when
	// bought").
	stroke(4, 0x01, 120)
	stroke(4, 0x01, 120) // second ENTER clears any leftover modal
	inkBoard := inkWin(8, 47, 12, 15)
	fmt.Printf("board after NEXT: jok[0]=%d ink joker-cell0=%d PC=$%04X\n",
		emu.mem.Read(jok), inkBoard, emu.cpu.PC)
	if inkBoard == 0 {
		t.Fatalf("F+10 bought but BOARD joker cell is EMPTY")
	}
}
