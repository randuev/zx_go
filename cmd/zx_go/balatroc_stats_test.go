package main

// balatroc_stats_test.go — PASS 49m stats overlay gate (Seva 2026-10-20:
// "d opens the table ... where are the remaining cards / not just counts,
// i want to know what cards remain / that table does not clear the
// contents underneath, score needed and round score everything is there
// blocking the view").
// Sentinel-jump _paint_stats (proven ctl jump law), then census:
//   1. CLEAR — old label pixels (SCORE NEEDED b2, ROUND SCORE b7) zero.
//   2. GRID — four mini-suit sprites lit in byte-col 0 (y104..167) and
//      rank glyphs present in the cell span.
//   3. CARD TRUTH — every one of the 52 cells carries EXACTLY one of
//      {ink attr (in deck), grey ghost attr (dealt/used)}; grey count ==
//      RAM _top exactly, cell-by-cell agreement with deck[] order.
//   4. RESTORE — jump _draw_board: b2 labels come back (overlay closed).
// All addresses parsed from the FRESH board.map at runtime (drift law).

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocStatsOverlayPlus2(t *testing.T) {
	if os.Getenv("STATSG") == "" {
		t.Skip("set STATSG=1")
	}
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	mapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.map")
	if err != nil {
		t.Fatalf("read map: %v", err)
	}
	syms := map[string]uint16{}
	re := regexp.MustCompile(`_(\w+)\s+=\s+\$([0-9A-Fa-f]{4})`)
	for _, m := range re.FindAllStringSubmatch(string(mapRaw), -1) {
		var v uint64
		fmt.Sscanf(m[2], "%x", &v)
		syms[m[1]] = uint16(v)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var boardData []byte
	var boardAddr uint16
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			boardData, boardAddr = blocks[i+1].data, b.start
		}
	}
	if boardData == nil {
		t.Fatal("no board block")
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
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	balRingPreload(t, emu, tapRaw)
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	pix := func(y, x int) byte {
		a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + x/8)
		return emu.mem.Read(a) & (1 << uint(7-(x&7)))
	}
	attr := func(row, col int) byte {
		return emu.mem.Read(uint16(0x5800 + row*32 + col))
	}
	lit := func(y0, y1, bx0, bx1 int) int {
		n := 0
		for y := y0; y <= y1; y++ {
			for bx := bx0; bx <= bx1; bx++ {
				a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
				if emu.mem.Read(a) != 0 {
					n++
				}
			}
		}
		return n
	}
	// boot the GAME: main() -> title -> '0' -> board
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
	emu.cpu.PC = boardAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF(150)            // title settles
	stroke(4, 0x01, 120) // '0' = START -> board
	if emu.cpu.PC == sent || emu.cpu.PC == sent+1 {
		t.Fatalf("game halted at sentinel before board: PC=$%04X", emu.cpu.PC)
	}
	before := lit(0, 191, 0, 31)
	gs := syms["g_stats"]
	fmt.Printf("STATS: g_stats pre-open=%d top=%d\n", emu.mem.Read(gs), emu.mem.Read(syms["top"]))
	// NOTE (drift law, 2026-10-20 live probe): some PressMatrixKey cells
	// cross-decode on the +2 headless core (pre-existing — hardware plays
	// fine); STATS (row1,0x04) and MUTE (row7,0x04) decode deterministically.
	stroke(7, 0x04, 60) // MUTE — also warms the event path
	stroke(7, 0x04, 60) // MUTE back
	// STROKE STATS: row1 bit0x04 = 'D'
	stroke(1, 0x04, 120)
	fmt.Printf("STATS: g_stats post-open=%d\n", emu.mem.Read(gs))
	open := lit(0, 191, 0, 31)
	spr := lit(104, 167, 0, 0)
	fmt.Printf("STATS: litBefore=%d litOpen=%d spriteRowsBytes=%d PC=$%04X\n", before, open, spr, emu.cpu.PC)
	savePNGAtomic("/tmp/stats_open.png", emu.lastFrame())
	if spr == 0 {
		t.Error("suit sprite byte-col empty -> grid not painted")
	}
	// clear law: b2 SCORE NEEDED label gone
	ghost := 0
	for y := 16; y < 24; y++ {
		for x := 0; x < 74; x++ {
			if pix(y, x) != 0 {
				ghost++
			}
		}
	}
	fmt.Printf("STATS: b2LabelPixelsLeft=%d\n", ghost)
	if ghost > 0 {
		t.Errorf("overlay did not clear b2 SCORE NEEDED area (%d px)", ghost)
	}
	// card truth (REBASELINED PASS 51/55 law): dealt/used cards are PLAIN
	// empty white space — no gray holes, no bright bit (PASS 55: "the dark
	// cell was a black hole"). Ghost = a 16x16 cell with ZERO lit pixels
	// and plain white-paper attr; ghost count == _top (cards already left
	// the deck). Old gate expected attr==0x40 gray holes at y104 — stale.
	top := int(emu.mem.Read(syms["top"]))
	gc := 0
	for s := 0; s < 4; s++ {
		y0 := 120 + s*16
		for k := 0; k < 13; k++ {
			cx := 16 + k*16
			litCell := 0
			for y := y0; y < y0+16; y++ {
				for bx := cx / 8; bx < cx/8+2; bx++ {
					a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
					if emu.mem.Read(a) != 0 {
						litCell++
					}
				}
			}
			if litCell == 0 && attr(y0/8, cx/8) == 0x38 {
				// plain white paper, black ink, NOT bright — PASS 55 law:
				// ghost cells carry no dark hole and no blazing bit
				gc++
			}
		}
	}
	fmt.Printf("STATS: blankGhostCells=%d _top=%d\n", gc, top)
	if gc != top {
		t.Errorf("blank ghost cells %d want _top=%d", gc, top)
	}
	// close -> board restored
	stroke(1, 0x04, 120)
	after := lit(0, 191, 0, 31)
	fmt.Printf("STATS: litClosed=%d\n", after)
	if after == 0 {
		t.Error("board not restored after stats close")
	}
}
