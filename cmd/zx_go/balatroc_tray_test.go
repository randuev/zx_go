package main

// balatroc_tray_test.go — PASS 67b TRAY gate (TRAY=1).
// Seva hardware: "i don't see tray strip with these icons, only when i
// press T". Law: owned tarots are visible ALL ROUND on b17 right half
// (px96..255): chip bx12 + name x104, second tarot chip bx21 + name
// x176. Direct-call census of resident paint_tray + negative control:
// the old tap painted band 11 INSIDE the cards and never persisted —
// its b17 census must be ZERO here (old tap fails).

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocTrayPlus2(t *testing.T) {
	if os.Getenv("TRAY") == "" {
		t.Skip("set TRAY=1")
	}
	tap := os.Getenv("ZX_TAP")
	if tap == "" {
		tap = "/root/nerve-workspace/demos/balatroc-c/build/board.tap"
	}
	mapPath := os.Getenv("ZX_MAP")
	if mapPath == "" {
		mapPath = "/root/nerve-workspace/demos/balatroc-c/build/board.map"
	}
	tapRaw, err := os.ReadFile(tap)
	if err != nil {
		t.Fatal(err)
	}
	mapRaw, _ := os.ReadFile(mapPath)
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
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	balRingPreload(t, emu, tapRaw)
	paintTray := syms["paint_tray"]
	if paintTray == 0 {
		t.Fatal("paint_tray missing from map")
	}
	// own MAGICIAN (id1) + TOWER (id4)
	emu.mem.Write(syms["tray"], 1)
	emu.mem.Write(syms["tray"]+1, 4)
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	// inline paging trampoline: LD A,0 / LD BC,$7FFD / OUT (C),A / JP
	// sentinel — page GAME (bank 0) into $C000 without balBankSel (whose
	// trampoline cannot run against a parked CPU).
	tr := []byte{0x3E, 0x00, 0x01, 0xFD, 0x7F, 0xED, 0x79, 0xC3, byte(sent), byte(sent >> 8)}
	for i, b := range tr {
		emu.mem.Write(0x7D10+uint16(i), b)
	}
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.PC = 0x7D10
	for i := 0; i < 16; i++ {
		if emu.cpu.PC == sent {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	if emu.cpu.PC != sent {
		t.Fatalf("paging trampoline stuck PC=$%04X", emu.cpu.PC)
	}
	emu.cpu.Halted = true // HALT at sentinel
	// direct call paint_tray: fresh return sentinel under it
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	emu.cpu.Halted = false
	emu.cpu.PC = paintTray
	for i := 0; i < 400000; i++ {
		if emu.cpu.PC == sent || emu.cpu.Halted {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	if emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
		t.Fatalf("paint_tray runaway PC=$%04X", emu.cpu.PC)
	}
	litAt := func(y0, y1, bx0, bx1 int) int {
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
	b11Before := litAt(88, 95, 11, 20)
	b18Before := litAt(144, 151, 12, 31)
	// re-call paint_tray (idempotence + differential)
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	emu.cpu.Halted = false
	emu.cpu.PC = paintTray
	for i := 0; i < 400000; i++ {
		if emu.cpu.PC == sent || emu.cpu.Halted {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	b17 := litAt(136, 143, 12, 31)
	b18 := litAt(144, 151, 12, 31)
	b11 := litAt(88, 95, 11, 20)
	fmt.Printf("TRAY: b17zone=%d b18=%d(before %d) b11=%d(before %d)\n",
		b17, b18, b18Before, b11, b11Before)
	if b17 == 0 {
		t.Error("b17 tray zone EMPTY — tarots not visible all round")
	}
	if b18 != 0 {
		t.Error("b18 dirty outside picker")
	}
	if b11 > b11Before {
		t.Errorf("paint_tray PAINTS b11 (%d->%d) — the old bug paints INSIDE the cards",
			b11Before, b11)
	}
	// picker highlight: armed slot0 plate over chip+name cells 12..20
	emu.mem.Write(syms["tray"], 0) // empty tray: fresh state for hop law
	emu.mem.Write(syms["tray"]+1, 0)
	savePNGAtomic("/tmp/tray_own2.png", emu.lastFrame())
}
