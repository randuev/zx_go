package main

// balatroc_f10_test.go — SEVA report 2026-10-18: "F+10 does not render
// when bought". Repro: boot, jump straight into shop_screen ($A377),
// force the left offer to id 11 (F+10), stroke SEL (the buy key), and
// census the owned-joker card pixels + jok[] state. PASS criteria:
// jok[0]==11 after buy AND the owned slot 0 renders a tag line (nonzero
// ink pixels in rows b17..b21, cols 1..6).

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
	const shopScreen = 0xA47E
	const shopID = 0xFA46
	const shopSold = 0xFA56
	const shopCost = 0xFA48
	const shopCur = 0xFA58
	const bank = 0xF7F3
	const jok = 0xFA3D
	// enter shop_screen with a fresh sentinel return
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
	runF(60) // shop paints; shop_new rolled two random offers
	fmt.Printf("shop offers: id0=%d id1=%d bank=%d\n",
		emu.mem.Read(shopID), emu.mem.Read(shopID+1), emu.mem.Read(bank))
	// FORCE left offer = F+10 (id 11), affordable, cursor on it
	emu.mem.Write(shopID, 11)
	emu.mem.Write(shopSold, 0)
	emu.mem.Write(shopCost, 4)
	emu.mem.Write(shopID+1, 4) // right offer away from 11 dupes
	emu.mem.Write(bank, 20)
	emu.mem.Write(shopCur, 0)
	// FULL repaint happens only on world change — force one via REROLL
	// is destructive; instead buy now: EV_SEL buys shop_id[0].
	inkBefore := 0
	for y := 136; y <= 175; y++ {
		for bx := 1; bx <= 6; bx++ {
			if emu.mem.Read(uint16(0x4000+(((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5))+bx)) != 0 {
				inkBefore++
			}
		}
	}
	// SEL key = row 4 bit 3 (bindings law in game_test header)
	emu.kbd.PressMatrixKey(4, 0x08, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x08, false)
	runF(80) // shop_paint full repaint after the buy
	j0 := emu.mem.Read(jok)
	inkAfter := 0
	for y := 136; y <= 175; y++ {
		for bx := 1; bx <= 6; bx++ {
			if emu.mem.Read(uint16(0x4000+(((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5))+bx)) != 0 {
				inkAfter++
			}
		}
	}
	fmt.Printf("after buy: jok[0]=%d bank=%d ink owned-slot0 before=%d after=%d PC=$%04X\n",
		j0, emu.mem.Read(bank), inkBefore, inkAfter, emu.cpu.PC)
	if j0 != 11 {
		t.Fatalf("BUY failed: jok[0]=%d want 11 (PC=$%04X)", j0, emu.cpu.PC)
	}
	if inkAfter == 0 {
		t.Fatalf("owned F+10 card renders NOTHING in slot 0")
	}
	// NEXT=[0]=(4,bit 0x01) leaves the shop -> next_round -> draw_board
	// -> draw_jokers. Census the BOARD joker cell (cols 12..15, b1..b5).
	emu.kbd.PressMatrixKey(4, 0x01, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	runF(120)
	inkBoard := 0
	for y := 8; y <= 47; y++ {
		for bx := 12; bx <= 15; bx++ {
			if emu.mem.Read(uint16(0x4000+(((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5))+bx)) != 0 {
				inkBoard++
			}
		}
	}
	fmt.Printf("board after NEXT: jok[0]=%d ink joker-cell0=%d PC=$%04X\n",
		emu.mem.Read(jok), inkBoard, emu.cpu.PC)
	if inkBoard == 0 {
		t.Fatalf("F+10 bought but BOARD joker cell is EMPTY")
	}
	// PNG ground truth of the shop-OWNED moment: rewind to shop, repaint
	emu.mem.Write(shopID, 11)
	emu.mem.Write(shopSold, 0)
	emu.mem.Write(shopCost, 4)
	emu.mem.Write(bank, 20)
	emu.kbd.PressMatrixKey(4, 0x08, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x08, false)
	runF(40)
	if err2 := savePNGAtomic("/tmp/f10_shop2.png", emu.lastFrame()); err2 != nil {
		fmt.Println("png fail:", err2)
	}
	// third buy -> slot 1
	emu.kbd.PressMatrixKey(4, 0x08, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x08, false)
	runF(40)
	fmt.Printf("jok=%d,%d,%d,%d,%d bank=%d\n", emu.mem.Read(jok),
		emu.mem.Read(jok+1), emu.mem.Read(jok+2), emu.mem.Read(jok+3),
		emu.mem.Read(jok+4), emu.mem.Read(bank))
	savePNGAtomic("/tmp/f10_shop3.png", emu.lastFrame())
}
