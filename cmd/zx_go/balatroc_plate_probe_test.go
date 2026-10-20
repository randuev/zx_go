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
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
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
	jumpA(syms["run_screen"], 1)
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
	rows := lit(88, 143, 0, 31)  // stats rows band
	fmt.Printf("PLATE win: headPxBytes=%d rowsBytes=%d PC=$%04X\n", head, rows, emu.cpu.PC)
	savePNGAtomic("/tmp/plate_win.png", emu.lastFrame())
	if head == 0 {
		t.Error("win plate big-letter band empty")
	}
	if rows == 0 {
		t.Error("win plate stats rows empty")
	}
	// OVER plate: wipe, wait_key returns instantly if a key is down — tap
	// one after paint so the function exits cleanly.
	emu.mem.Write(syms["run_best_ht"], 0)
	emu.mem.Write(syms["top"], 41)
	jumpA(syms["run_screen"], 0)
	runF(240)
	head = lit(40, 71, 10, 25)
	fmt.Printf("PLATE over: headPxBytes=%d PC=$%04X\n", head, emu.cpu.PC)
	savePNGAtomic("/tmp/plate_over.png", emu.lastFrame())
	if head == 0 {
		t.Error("over plate big-letter band empty")
	}
}
