package main

// balatroc_plus2a_probe_test.go — PROBE2A=1: does the SHIPPED (pass-67b)
// boot survive on the +2A/+3 memory map, or only classic +2?
//
// Static reading of the MMU: zx_go PageMemory decodes the $C000 RAM window
// from bits 0-2 of $7FFD on ALL models (RAM paging OK on +2A/+3), but
// selectROM on Plus2A/Plus3 combines $7FFD bit 4 (low) with $1FFD bit 2
// (high): the 48 BASIC ROM is index 1 on classic +2 but index 3 on +2A/+3.
// balatroc ORs only bit 4 and NEVER touches $1FFD.
//
// The board.tap deck is PRELOADED by the harness (balRingPreload), so
// boot_pages' idempotent door-gate skips all CALL $0556 tape loads on all
// three models — the wrong-ROM risk is only live on a REAL tape boot, which
// headless cannot feed. This probe answers:
//   1. boot through the real entry $8000 (crt0) on each model;
//   2. which ROM index+bytes end up at $0000 after boot_pages' OUTs;
//   3. does the title poll stay alive (PC stays in game space, no sentinel,
//      no escape to ROM) — and do the ROM bytes match classic's 48 ROM.
// Plus2 runs first as the passing control; +2A/+3 report vs it.
import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func balRunProbe(t *testing.T, model roms.SpectrumModel, tag string, frames int) (romBytes []byte, crashPC uint16, crashed bool, alive bool) {
	t.Helper()
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd, md []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			md = blocks[i+1].data
		}
	}
	if bd == nil {
		t.Fatal("no board block")
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(model)
	if err != nil {
		t.Fatalf("%s: model unavailable: %v", tag, err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, model)
	}
	fmt.Printf("[%s] boot menu ROM@0556: %02X %02X %02X %02X\n", tag,
		emu.mem.Read(0x0556), emu.mem.Read(0x0557), emu.mem.Read(0x0558), emu.mem.Read(0x0559))
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	if md != nil {
		for i, v := range md {
			emu.mem.Write(0x6000+uint16(i), v)
		}
	}
	balRingPreload(t, emu, tapRaw)

	// ROM select witness: record the first 8 ROM slot changes
	romLog := []string{}
	emu.mem.SetBankTracer(func(slot uint8, newBank int, source string) {
		if slot == 0 && len(romLog) < 8 {
			romLog = append(romLog, fmt.Sprintf("rom%d@%s", newBank, source))
		}
	})

	// boot through the REAL entry: crt0 at $8000, exactly like RANDOMIZE USR 32768
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.Halted = false
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	// run frames; crash = escape to ROM (PC<$4000) or sentinel hit
	inRomFirst := uint16(0)
	lowHits := 0
	for f := 0; f < frames; f++ {
		runOneFrameHeadless(emu, model)
		pc := emu.cpu.PC
		if pc == sent {
			crashed = true
			crashPC = pc
			fmt.Printf("[%s] SENTINEL hit f=%d SP=$%04X events=%v\n", tag, f, emu.cpu.SP, romLog)
			break
		}
		if pc < 0x4000 {
			lowHits++
			if lowHits <= 3 {
				ret := uint16(emu.mem.Read(emu.cpu.SP)) | uint16(emu.mem.Read(emu.cpu.SP+1))<<8
				fmt.Printf("[%s] LOW-PC f=%d PC=$%04X SP=$%04X ret=$%04X events=%v\n", tag, f, pc, emu.cpu.SP, ret, romLog)
			}
			if inRomFirst == 0 {
				inRomFirst = pc
			}
		}
	}
	crashPC = inRomFirst
	romBytes = []byte{emu.mem.Read(0x0556), emu.mem.Read(0x0557), emu.mem.Read(0x0558), emu.mem.Read(0x0559)}
	alive = !crashed && lowHits == 0
	fmt.Printf("[%s] done: frames=%d lowPC-hits=%d ROM@0556=%02X %02X %02X %02X events=%v PC=$%04X\n",
		tag, frames, lowHits, romBytes[0], romBytes[1], romBytes[2], romBytes[3], romLog, emu.cpu.PC)
	return
}

func TestBalatrocPlus2AProbe(t *testing.T) {
	if os.Getenv("PROBE2A") == "" {
		t.Skip("set PROBE2A=1")
	}
	cROM, cPC, cCrash, cLive := balRunProbe(t, roms.ModelPlus2, "plus2-classic", 80)
	if cCrash {
		t.Fatalf("CONTROL plus2 crashed at sentinel — baseline broken")
	}
	if !cLive {
		t.Errorf("CONTROL plus2 escaped to ROM $%04X — baseline broken", cPC)
	}
	aROM, aPC, aCrash, aLive := balRunProbe(t, roms.ModelPlus2A, "plus2A", 80)
	pROM, pPC, pCrash, pLive := balRunProbe(t, roms.ModelPlus3, "plus3", 80)

	fmt.Printf("PROBE2A summary:\n classic ROM@0556=% X live=%v\n +2A    ROM@0556=% X same-as-classic=%v live=%v crashSentinel=%v lowPC=$%04X\n +3     ROM@0556=% X same-as-classic=%v live=%v crashSentinel=%v lowPC=$%04X\n",
		cROM, cLive, aROM, string(aROM) == string(cROM), aLive, aCrash, aPC,
		pROM, string(pROM) == string(cROM), pLive, pCrash, pPC)
}
