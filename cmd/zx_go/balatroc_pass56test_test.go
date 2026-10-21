package main

// PASS 56 ring boot-truth gate: boot from the REAL tape path — header
// parse, load CORE into fixed banks, patch nothing (tape leads C3), set
// the loader's BASIC state (RAMTOP $BFFB/SP $BFF5), jump into core, and
// PROVE main() runs: resident lives at $8000 (DI/crt head), frames tick,
// no ROM takeover. This is the tape-on-hardware path minus the tape deck.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocPass56BootPlus2(t *testing.T) {
	tap := os.Getenv("BOOT56")
	if tap == "" {
		t.Skip("set BOOT56=<tap>")
	}
	raw, err := os.ReadFile(tap)
	if err != nil {
		t.Fatal(err)
	}
	blocks := balParseTapBlocks(t, raw)
	var core []byte
	var pages = map[string][]byte{}
	for i := range blocks {
		b := blocks[i]
		if !b.hdr || b.typ != 3 || i+1 >= len(blocks) {
			continue
		}
		switch trimName(b.name) {
		case "core":
			core = blocks[i+1].data
		case "page_game":
			pages["game"] = blocks[i+1].data
		case "page_shop":
			pages["shop"] = blocks[i+1].data
		}
	}
	if core == nil || len(pages) != 2 {
		t.Fatalf("blocks: core=%v pages=%d", core != nil, len(pages))
	}
	if core[0] != 0xF3 {
		t.Errorf("core head %02X != F3 (DI crt)", core[0])
	}
	for _, p := range []string{"game", "shop"} {
		if pages[p][0] != 0xC3 {
			t.Errorf("page %s head %02X != C3 (door_vec)", p, pages[p][0])
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
	// loader state: RAMTOP $BFFB, SP under it, USR sentinel
	emu.mem.Write(0x5CB2, 0xFB)
	emu.mem.Write(0x5CB3, 0xBF)
	for i, v := range core {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	balRingPreload(t, emu, raw)
	balBankSel(t, emu, 0)
	if emu.mem.Read(0xC000) != 0xC3 || emu.mem.Read(0x8000) != 0xF3 {
		t.Fatalf("map @OUT0: win=%02X core=%02X", emu.mem.Read(0xC000), emu.mem.Read(0x8000))
	}
	balBankSel(t, emu, 1)
	if emu.mem.Read(0xC000) != 0xC3 {
		t.Fatalf("map @OUT1: win=%02X", emu.mem.Read(0xC000))
	}
	balBankSel(t, emu, 2)
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	romTaken := 0
	pcLoop := 0
	for f := 0; f < 300; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		pc := emu.cpu.PC
		if pc < 0x4000 {
			romTaken++
			if romTaken == 1 {
				fmt.Printf("BOOT56 ROM takeover f=%d last=%04X SP=%04X\n", f, pc, emu.cpu.SP)
			}
		} else if pc >= 0x8000 {
			pcLoop++
		}
		if romTaken == 0 && pcLoop > 100 && f == 299 {
			fmt.Printf("BOOT56 ALIVE pc=%04X SP=%04X\n", pc, emu.cpu.SP)
		}
	}
	if romTaken > 0 {
		t.Errorf("ROM takeover %d frames — crash out of core", romTaken)
	}
	if pcLoop == 0 {
		t.Error("never in resident code")
	}
}
