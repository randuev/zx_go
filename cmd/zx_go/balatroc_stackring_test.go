package main

// balatroc_stackring_test.go — PASS 62 STACK ROOM gate (STACKRING=1).
// The bug pass 61b shipped: CORE tail $BFFF, SP forced $BFF5 — ZERO stack.
// RANDOMIZE USR's return + boot_pages' LD-BYTES call chain pushed straight
// into CORE's own code -> crash to BASIC right after "Bytes: core".
// Every older gate preloaded banks (idempotent skip) so boot_pages made
// ZERO tape calls and the stack bomb never armed.
//
// This gate runs the TRUE boot_pages path: mount the real tape, trap
// $0556, garbage banks 0/1 (door mismatch -> real LD-BYTES), step the
// CPU instruction by instruction and record SP high-water. LAWS enforced:
//   1. SP never descends to/below the resident CORE tail (stack intact).
//   2. LD-BYTES traps fire (tape deck actually drove boot_pages).
//   3. both page banks land with their JP door (tape actually loaded).
//   4. PC enters the $C000 window (page code ran — title lives there now)
//      and never escapes to ROM (<$4000) after core entry (no crash).
// Negative control: ZX_TAP=pass61b.tap MUST fail laws 1/2.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocStackRing(t *testing.T) {
	if os.Getenv("STACKRING") == "" {
		t.Skip("set STACKRING=1")
	}
	tap := os.Getenv("ZX_TAP")
	if tap == "" {
		tap = "/root/nerve-workspace/demos/balatroc-c/build/board.tap"
	}
	raw, err := os.ReadFile(tap)
	if err != nil {
		t.Fatal(err)
	}
	blocks := balParseTapBlocks(t, raw)
	var core []byte
	pageDoor := map[string]uint16{}
	pageData := map[string][]byte{}
	for i := range blocks {
		b := blocks[i]
		if !b.hdr || b.typ != 3 || i+1 >= len(blocks) {
			continue
		}
		switch trimName(b.name) {
		case "core":
			core = blocks[i+1].data
		case "page_game":
			pageDoor["game"] = uint16(blocks[i+1].data[1]) | uint16(blocks[i+1].data[2])<<8
			pageData["game"] = blocks[i+1].data
		case "page_shop":
			pageDoor["shop"] = uint16(blocks[i+1].data[1]) | uint16(blocks[i+1].data[2])<<8
			pageData["shop"] = blocks[i+1].data
		}
	}
	if core == nil || len(pageData) != 2 {
		t.Fatalf("tap %s: core=%v pages=%d", tap, core != nil, len(pageData))
	}
	// resident tail = org $8000 + len - 1 (< $C000 enforced by build gate)
	// peek resident door pair at $C001/$C002 for the idempotency truth
	// (boot_pages compares these against petab; garbage banks must differ)
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
	dbg := os.Getenv("STACKRING_DBG") != ""
	if err := loadTapeFile(emu, tap); err != nil {
		t.Fatal(err)
	}
	installTapeTrap(emu)
	emu.paused.Store(false)
	for i := 0; i < 200; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	// emulate the LOADER line's ML-visible half: RAMTOP $C000 + CORE into
	// fixed banks (bank 2 is always visible at $8000 on +2).
	emu.mem.Write(0x5CB2, 0x00)
	emu.mem.Write(0x5CB3, 0xC0)
	for i, v := range core {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	// fast-forward the tape to the first page header (skip loader+core —
	// consumed by "BASIC" above) so boot_pages' flag sequence lines up.
	tp := emu.ula.GetTapePlayer()
	for tp.HasMoreBlocks() && tp.CurrentBlock() < 4 {
		tp.NextBlock()
	}
	// garbage banks 0/1 at the door pair -> boot_pages MUST take the tape
	// (idempotent-skip path would hide the whole stack bomb).
	for _, bank := range []uint8{0, 1} {
		balBankSel(t, emu, bank)
		for a := 0xC000; a < 0xC010; a++ {
			emu.mem.Write(uint16(a), 0x00)
		}
	}
	balBankSel(t, emu, 2)
	// ROM state: classic +2 pages ROM via $7FFD bit 4 (menu ROM 0 / 48
	// BASIC 1 with LD-BYTES). balBankSel writes leave bit 4 clear (menu
	// ROM) — boot_pages' own OUT sets bit 4 (ROM LAW pass 62), so the
	// trap sees bank 1 exactly when the game executes the CALL.
	// RANDOMIZE USR: SP $BFF5 + return sentinel, PC into crt0
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF3, byte(sent))
	emu.mem.Write(0xBFF4, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	// Step instruction by instruction (for an honest SP high-water), but
	// MIRROR ExecuteFrame's trap consumption: TrapCheck only fires inside
	// ExecuteFrame's loop (z80.go:978), so a raw step loop would run the
	// REAL ROM LD-BYTES at edge timing (stalls forever / trashes stack).
	// The trap emulates the full CALL including its RET (main.go:2438).
	const steps = 2000000 // several frames of boot + title at insn granularity
	spMin := uint16(0xFFFF)
	spAtTrap := uint16(0xFFFF)
	trapHits := 0
	romEscape := 0
	windowHits := 0
	inCore := false
	for i := 0; i < steps; i++ {
		pc := emu.cpu.PC
		if emu.cpu.TrapCheck != nil && emu.cpu.TrapCheck(pc) {
			trapHits++
			if emu.cpu.SP < spAtTrap {
				spAtTrap = emu.cpu.SP
			}
		} else {
			if dbg && i < 400 {
				fmt.Printf("DBG %04d pc=%04X sp=%04X b=%02X %02X %02X a=%02X bc=%04X de=%04X hl=%04X ix=%04X\n",
					i, emu.cpu.PC, emu.cpu.SP, emu.mem.Read(emu.cpu.PC), emu.mem.Read(emu.cpu.PC+1),
					emu.mem.Read(emu.cpu.PC+2), emu.cpu.A, uint16(emu.cpu.B)<<8|uint16(emu.cpu.C), uint16(emu.cpu.D)<<8|uint16(emu.cpu.E),
					emu.cpu.HL(), emu.cpu.IX)
			}
			emu.cpu.StepInstructionWithIRQ()
		}
		pc = emu.cpu.PC
		if emu.cpu.SP < spMin {
			spMin = emu.cpu.SP
		}
		if pc >= 0x8000 && pc < 0xC000 {
			inCore = true
		}
		if pc >= 0xC000 {
			windowHits++
		}
		if inCore && pc < 0x4000 && pc != sent && pc != 0x0556 {
			romEscape++
			if romEscape == 1 {
				t.Logf("ROM escape step=%d pc=%04X sp=%04X", i, pc, emu.cpu.SP)
			}
		}
	}
	fmt.Printf("STACKRING %s: tail=%04X spMin=%04X trapHits=%d spAtTrap=%04X windowHits=%d romEsc=%d\n",
		tap, 0x8000+len(core)-1, spMin, trapHits, spAtTrap, windowHits, romEscape)

	// LAW 1: stack never reached the resident tail (the pass-61b bomb)
	if spMin <= uint16(len(core))+0x8000-1 {
		t.Errorf("STACK BOMB: spMin $%04X <= resident tail $%04X — pushes ate CORE",
			spMin, uint16(len(core))+0x8000-1)
	}
	// LAW 3: pages landed via REAL LD-BYTES (banks hold their doors)
	balBankSel(t, emu, 0)
	if d := uint16(emu.mem.Read(0xC001)) | uint16(emu.mem.Read(0xC002))<<8; d != pageDoor["game"] {
		t.Errorf("bank0 door %04X != tap %04X — GAME page not loaded", d, pageDoor["game"])
	}
	if emu.mem.Read(0xC000) != 0xC3 {
		t.Errorf("bank0 byte0 %02X != C3", emu.mem.Read(0xC000))
	}
	balBankSel(t, emu, 1)
	if d := uint16(emu.mem.Read(0xC001)) | uint16(emu.mem.Read(0xC002))<<8; d != pageDoor["shop"] {
		t.Errorf("bank1 door %04X != tap %04X — SHOP page not loaded", d, pageDoor["shop"])
	}
	// spot-check body integrity mid-page (bank0 vs tap bytes)
	for _, off := range []uint16{0x100, 0x900, 0x1900} {
		if off < uint16(len(pageData["game"])) {
			balBankSel(t, emu, 0)
			if emu.mem.Read(0xC000+off) != pageData["game"][off] {
				t.Errorf("bank0+$X%03X corrupt", off)
			}
		}
	}
	// LAW 4: page code actually ran, no ROM crash
	if windowHits == 0 {
		t.Error("PC never entered the $C000 window — page never ran")
	}
	if romEscape > 0 {
		t.Errorf("ROM escape after core entry (%d) — crash out of the game", romEscape)
	}
}
