package main

// balatroc_reorder_test.go — task store-joker-reorder-overprint gate:
// PASS 50 found the 48d wipe left the FRAME BYTES bx/bx+5 un-erased;
// centered tag text (offset (48-tlen*6)/2 lands inside byte bx) survived
// a swap as edge scratches = "jokers drawn on top of each other".
// Law proved here = REPAINT IDEMPOTENCE: paint slot0 with joker X,
// repaint as joker Y, repaint back as X — the card-column pixel block
// must be byte-identical to the first paint (no history residue).
// Negative control: run with ZX_TAP=<HEAD tap> — MUST FAIL (residue).

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocReorderPlus2(t *testing.T) {
	if os.Getenv("REORDG") == "" {
		t.Skip("set REORDG=1")
	}
	tap := os.Getenv("ZX_TAP")
	if tap == "" {
		tap = "/root/nerve-workspace/demos/balatroc-c/build/board.tap"
	}
	tapRaw, err := os.ReadFile(tap)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
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
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	call1 := func(fn uint16, a byte) {
		emu.cpu.Halted = false
		// sdcc_iy arg law (asm-proven): caller pushes ARG first, then
		// CALL pushes the return; callee reads the byte at (ix+4).
		emu.cpu.SP -= 2
		emu.mem.Write(emu.cpu.SP, a)
		emu.mem.Write(emu.cpu.SP+1, 0)
		emu.cpu.SP -= 2
		emu.mem.Write(emu.cpu.SP, byte(sent))
		emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
		emu.cpu.PC = fn
		g := 0
		for emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
			emu.cpu.StepInstructionWithIRQ()
			if g++; g > 4_000_000 {
				t.Fatal("owned_card runaway")
			}
		}
	}
	// owned row: two jokers, very different tag lengths (4 vs 6 chars)
	jok := syms["jok"]
	emu.mem.Write(jok, 19) // STCK
	emu.mem.Write(jok+1, 15) // DECKIE
	// capture slot 0 column bytes bx=1..6, rows 136..175
	bx := 1
	cap0 := func() []byte {
		out := make([]byte, 0, 40*6)
		for y := 136; y <= 175; y++ {
			a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
			for k := 0; k < 6; k++ {
				out = append(out, emu.mem.Read(a+uint16(k)))
			}
		}
		return out
	}
	call1(syms["owned_card"], 0)
	pX := cap0()
	// swap occupants and repaint slot 0 (now DECKIE)
	emu.mem.Write(jok, 15)
	emu.mem.Write(jok+1, 19)
	call1(syms["owned_card"], 0)
	pY := cap0()
	// swap back — repaint must be byte-identical to pX (no residue)
	emu.mem.Write(jok, 19)
	emu.mem.Write(jok+1, 15)
	call1(syms["owned_card"], 0)
	pX2 := cap0()
	diff := 0
	for i := range pX {
		if pX[i] != pX2[i] {
			diff++
		}
	}
	fmt.Printf("REORDER: paintX bytes=%d lit=%d  afterXY->X residueBytes=%d\n",
		len(pX), cntLit(pX), diff)
	if diff == 0 {
		if bytesEq(pX, pY) {
			t.Error("paint Y identical to X — repaint did nothing (gate broken)")
		}
	} else {
		t.Errorf("residue after swap-back: %d bytes differ — overprint not erased", diff)
	}
}

func cntLit(b []byte) int {
	n := 0
	for _, v := range b {
		if v != 0 {
			n++
		}
	}
	return n
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
