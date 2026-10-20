package main

// balatroc_wincheck_test.go — task 2026-10-18-false-blind-defeated gate:
// false BLIND DEFEATED at 1196/90000 (ante 6 small). The pass-47 bug
// divided tgt itself to ZERO while the HUD printed from a copy — every
// play beat any ante>=5 target. Direct-call law (proven): jump
// _compute_target (A ignored), read the REAL _tgt u32 from RAM after
// TWO calls (idempotence = the stringify consumes a copy), assert
// ante6-small == 90000 and 1196 < tgt. Negative control: run against
// git HEAD tap must PASS too (fix is in HEAD) — the gate guards against
// regression of the `v = tgt` copy law.

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocWinCheckPlus2(t *testing.T) {
	if os.Getenv("WING") == "" {
		t.Skip("set WING=1")
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
	rd32 := func(a uint16) uint32 {
		return uint32(emu.mem.Read(a)) | uint32(emu.mem.Read(a+1))<<8 |
			uint32(emu.mem.Read(a+2))<<16 | uint32(emu.mem.Read(a+3))<<24
	}
	callCT := func() {
		emu.cpu.Halted = false
		emu.cpu.SP -= 2
		emu.mem.Write(emu.cpu.SP, byte(sent))
		emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
		emu.cpu.PC = syms["compute_target"]
		for emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
			emu.cpu.StepInstructionWithIRQ()
		}
	}
	// ante 6 SMALL blind — the exact false-win config from the photo
	emu.mem.Write(syms["ante"], 6)
	emu.mem.Write(syms["blind"], 0)
	callCT()
	t1 := rd32(syms["tgt"])
	callCT() // idempotence: second call must NOT shrink tgt
	t2 := rd32(syms["tgt"])
	fmt.Printf("WINCHECK ante6-small: tgt=%d after2nd=%d\n", t1, t2)
	if t1 != 90000 {
		t.Errorf("tgt ante6-small=%d want 90000", t1)
	}
	if t2 != 90000 {
		t.Errorf("2nd call shrank tgt to %d — stringify eats tgt (pass-47 regression)", t2)
	}
	// score 1196 (photo) must NOT beat it; compare the real operands
	score := uint32(1196)
	if score >= t2 {
		t.Error("false win: 1196 >= tgt")
	}
	// boss ante6 must be bigger than small
	emu.mem.Write(syms["blind"], 2)
	callCT()
	t3 := rd32(syms["tgt"])
	fmt.Printf("WINCHECK ante6-boss: tgt=%d\n", t3)
	if t3 <= t2 {
		t.Errorf("boss tgt %d !> small %d", t3, t2)
	}
	// endless climb: ante 7 small = ante6 boss doubled
	emu.mem.Write(syms["ante"], 7)
	emu.mem.Write(syms["blind"], 0)
	callCT()
	t4 := rd32(syms["tgt"])
	fmt.Printf("WINCHECK ante7-small: tgt=%d\n", t4)
	if t4 != t3*2 {
		t.Errorf("endless climb ante7-small=%d want %d", t4, t3*2)
	}
}
