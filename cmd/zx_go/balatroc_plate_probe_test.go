package main

// balatroc_plate_probe_test.go — throwaway visual capture (RUNPLATE=1):
// boots the real game, sets a plausible WIN state, direct-jumps
// _run_screen(1) (A=1, sentinel HALT return — proven ctl-jump law),
// saves PNG; then repeats with dirge/OVER state. Visual proof only;
// assertions = lit-pixel presence in plate bands.

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocRunPlateProbe(t *testing.T) {
	if os.Getenv("RUNPLATE") == "" {
		t.Skip("set RUNPLATE=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	mapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.map")
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
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	jumpA := func(fn uint16, a byte) {
		// sdcc_iy arg law (asm-proven): caller pushes ARG first, then
		// the return address; callee reads (ix+4).
		emu.cpu.Halted = false
		emu.cpu.SP -= 2
		emu.mem.Write(emu.cpu.SP, a)
		emu.mem.Write(emu.cpu.SP+1, 0)
		emu.cpu.SP -= 2
		emu.mem.Write(emu.cpu.SP, byte(sent))
		emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
		emu.cpu.PC = fn
	}
	runF(150)
	// WIN state: ante 6 big blind beat, some run stats, 2 jokers, 17 left
	wr := func(n string, off int, v byte) { emu.mem.Write(syms[n]+uint16(off), v) }
	emu.mem.Write(syms["ante"], 6)
	emu.mem.Write(syms["blind"], 2)
	emu.mem.Write(syms["round_no"], 12)
	wr("run_score", 0, 0x10)
	wr("run_score", 1, 0x27) // 10000
	wr("run_best_score", 0, 0xA0)
	wr("run_best_score", 1, 0x86) // 34880... whatever
	wr("run_best_ht", 0, 6)
	wr("run_best_lvl", 0, 3)
	emu.mem.Write(syms["top"], 17)
	wr("jok", 0, 3)
	wr("jok", 1, 7)
	balBankSel(t, emu, 1)
	jumpA(syms["run_screen"], 1)
	// TEMP TRACE: find where the jump escapes into ROM
	for i := 0; i < 60000; i++ {
		pcB := emu.cpu.PC
		emu.cpu.StepInstructionWithIRQ()
		if emu.cpu.PC < 0x8000 && pcB >= 0x8000 {
			fmt.Printf("TRACE escape: fromPC=$%04X -> $%04X (insn idx %d)\n",
				pcB, emu.cpu.PC, i)
			break
		}
		if emu.cpu.PC == sent || emu.cpu.Halted {
			fmt.Printf("TRACE parked at $%04X i=%d\n", emu.cpu.PC, i)
			break
		}
	}
	runF(240) // ~4 s of plate incl. sfx fanfare pumps
	if emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
		t.Logf("plate win did not park on sentinel yet: PC=$%04X", emu.cpu.PC)
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
	head := lit(40, 71, 10, 25) // big-letter band
	rows := lit(88, 143, 0, 31) // stats rows band
	fmt.Printf("PLATE win: headPxBytes=%d rowsBytes=%d PC=$%04X\n", head, rows, emu.cpu.PC)
	savePNGAtomic("/tmp/plate_win.png", emu.lastFrame())
	if head == 0 {
		t.Error("win plate big-letter band empty")
	}
	if rows == 0 {
		t.Error("win plate stats rows empty")
	}
	// PASS 67 (Seva: "win screen has red colour only on portion of big
	// letters"): text12 pitch is 24 px = 3 attr cells per glyph; the old
	// attrrect covered only the middle 14 cols. Exact span law — the
	// big-letter attr (ink CYAN for WIN / RED for OVER on black paper)
	// must cover EVERY cell the doubled text lands on: "YOU WIN!"
	// (8 glyphs @ x=32): px 32..223 -> attr cols 4..27; "GAME OVER"
	// (9 @ x=20): px 20..235 -> cols 2..29. Cell just OUTSIDE must not
	// carry the ink.
	inkOf := func(row, col int) int {
		return int(emu.mem.Read(uint16(0x5800+row*32+col)) & 7)
	}
	spanCheck := func(tag string, rows []int, c0, c1, ink int) {
		for _, r := range rows {
			for c := c0; c <= c1; c++ {
				if inkOf(r, c) != ink {
					t.Errorf("%s: attr r%d c%d ink=%d want %d — big-letter colour span short",
						tag, r, c, inkOf(r, c), ink)
					return
				}
			}
			if inkOf(r, c1+1) == ink {
				t.Errorf("%s: attr r%d c%d bleeds ink %d past span", tag, r, c1+1, ink)
			}
		}
	}
	spanCheck("WIN", []int{6, 7}, 4, 27, 5) // CYAN ink (=5)
	fmt.Printf("PLATE win attr ink: r6c3=%d r6c4=%d r6c27=%d r6c28=%d\n",
		inkOf(6, 3), inkOf(6, 4), inkOf(6, 27), inkOf(6, 28))
	// OVER plate: wipe, wait_key returns instantly if a key is down — tap
	// one after paint so the function exits cleanly.
	emu.mem.Write(syms["run_best_ht"], 0)
	emu.mem.Write(syms["top"], 41)
	balBankSel(t, emu, 1)
	jumpA(syms["run_screen"], 0)
	runF(240)
	head = lit(40, 71, 10, 25)
	fmt.Printf("PLATE over: headPxBytes=%d PC=$%04X\n", head, emu.cpu.PC)
	spanCheck("OVER", []int{6, 7}, 2, 29, 2) // RED ink (=2)
	fmt.Printf("PLATE over attr ink: r6c1=%d r6c2=%d r6c29=%d r6c30=%d\n",
		inkOf(6, 1), inkOf(6, 2), inkOf(6, 29), inkOf(6, 30))
	savePNGAtomic("/tmp/plate_over.png", emu.lastFrame())

	// RAM headline census: ink columns must separate chars at pitch 24
	cols := make([]int, 256)
	for y := 48; y < 62; y++ {
		for bx := 0; bx < 32; bx++ {
			a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
			v := emu.mem.Read(a)
			for b := 0; b < 8; b++ {
				if v&(1<<uint(7-b)) != 0 {
					cols[bx*8+b]++
				}
			}
		}
	}
	// runs of lit columns
	var runs [][2]int
	inr := false
	st := 0
	for x := 0; x < 256; x++ {
		if cols[x] > 0 && !inr {
			st, inr = x, true
		}
		if cols[x] == 0 && inr {
			runs = append(runs, [2]int{st, x - st})
			inr = false
		}
	}
	fmt.Printf("PLATE cols runs=%v\n", runs)

	if head == 0 {
		t.Error("over plate big-letter band empty")
	}
}
