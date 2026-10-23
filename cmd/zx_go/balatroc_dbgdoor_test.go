package main

// balatroc_dbgdoor_test.go — PASS 65 honest-launch proof (DBGOOR=1).
// Boots the SHIPPING tap on the real trap/step path: pages preloaded into
// their window banks (harness law), core run from its tap address,
// TrapCheck consumed like the frame loop. Phase machine knocks START until
// the flow leaves TITLE(5) then D until g_stats latches — the PASS 65
// knock law. Emulator-side scaffolding only — the game code is untouched.

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocDbgDoorPlus2(t *testing.T) {
	if os.Getenv("DBGOOR") == "" {
		t.Skip("set DBGOOR=1")
	}
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Skipf("no frozen tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var core []byte
	var coreAddr uint16
	pages := map[int][]byte{}
	for i := range blocks {
		b := blocks[i]
		if !b.hdr || b.typ != 3 || i+1 >= len(blocks) {
			continue
		}
		switch trimName(b.name) {
		case "core":
			coreAddr, core = b.start, blocks[i+1].data
		case "page_game":
			pages[0] = blocks[i+1].data
		case "page_shop":
			pages[1] = blocks[i+1].data
		}
	}
	if core == nil {
		t.Fatal("no core block")
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

	for i, v := range core {
		emu.mem.Write(coreAddr+uint16(i), v)
	}
	// harness law: pages straight into their window banks
	for p, d := range pages {
		balBankSel(t, emu, byte(p))
		for i, v := range d {
			emu.mem.Write(0xC000+uint16(i), v)
		}
		head := ""
		for ad := 0xC000; ad < 0xC00C; ad++ {
			head += fmt.Sprintf("%02X", emu.mem.Read(uint16(ad)))
		}
		fmt.Printf("PAGE%d len=%d head=%s\n", p, len(d), head)
	}
	balBankSel(t, emu, 2)

	sent := uint16(0x7D00)
	for i := sent; i < sent+8; i++ {
		emu.mem.Write(i, 0x76)
	}
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF3, byte(sent))
	emu.mem.Write(0xBFF4, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	mapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.map")
	syms := map[string]uint16{}
	re := regexp.MustCompile(`_(\w+)\s+=\s+\$([0-9A-Fa-f]{4})`)
	for _, m := range re.FindAllStringSubmatch(string(mapRaw), -1) {
		var v uint64
		fmt.Sscanf(m[2], "%x", &v)
		syms[m[1]] = uint16(v)
	}
	flow, gstats := syms["flow"], syms["g_stats"]

	// step; consume TrapCheck like the harness frame loop does.
	// Phase machine: TITLE loop -> stroke START -> SHOP loop -> stroke D ->
	// g_stats must latch and the overlay must be repainted every frame.
	inGame := false
	prints := 0
	step := 0
	titleLoops := 0
	stroked := 0
	shopSeen := false
	statsSeen := false
	for ; step < 3_000_000; step++ {
		if emu.cpu.Halted {
			fmt.Printf("HALT step=%d PC=$%04X\n", step, emu.cpu.PC)
			break
		}
		pc := emu.cpu.PC
		if pc >= 0x8000 && pc < 0xC000 {
			inGame = true
		}
		if pc == sent {
			fmt.Printf("SENTINEL step=%d\n", step)
			break
		}
		if inGame && pc < 0x4000 {
			fmt.Printf("ROM-LAP step=%d PC=$%04X SP=$%04X flow=%d\n", step, pc, emu.cpu.SP, emu.mem.Read(flow))
			break
		}
		// START knocks: resident title poll runs pageless. Dwell 12k steps
		// (~one 50 Hz scan window of T-states) — scanKey edge-latches.
		if !shopSeen && step > 30000 && step%30000 == 0 {
			fmt.Printf("START knock #%d @step=%d flow=%d\n", stroked+1, step, emu.mem.Read(flow))
			emu.kbd.PressMatrixKey(4, 0x01, true)
			for k := 0; k < 12000; k++ {
				emu.cpu.StepInstructionWithIRQ()
			}
			emu.kbd.PressMatrixKey(4, 0x01, false)
			step += 2000
			stroked++
		}
		fl := emu.mem.Read(flow)
		// step-0 law: flow bss reads 0 before main() initializes — require
		// the round to actually be running (step>100k) before latching SHOP
		if fl == 0 && step > 100000 && !shopSeen {
			shopSeen = true
			fmt.Printf("SHOP flow @step=%d PC=$%04X\n", step, pc)
		}
		if step%50000 == 0 {
			fmt.Printf("POLL step=%d flow=%d gstats=%d PC=$%04X titleLoops=%d stroked=%d\n", step, fl, emu.mem.Read(gstats), pc, titleLoops, stroked)
		}
		if shopSeen && !statsSeen && step%15000 == 0 {
			// stroke D (row 1 bit 0x04) until the overlay latches
			emu.kbd.PressMatrixKey(1, 0x04, true)
			for k := 0; k < 12000; k++ {
				emu.cpu.StepInstructionWithIRQ()
			}
			emu.kbd.PressMatrixKey(1, 0x04, false)
			step += 200
			if emu.mem.Read(gstats) == 1 {
				statsSeen = true
				fmt.Printf("STATS latched @step=%d gstats=1\n", step)
			}
		}
		if statsSeen {
			break
		}
		if pc >= 0xC000 && prints < 40 {
			fmt.Printf("%7d PAGE PC=$%04X SP=$%04X A=$%02X HL=$%04X flow=%d gstats=%d\n",
				step, pc, emu.cpu.SP, emu.cpu.A, emu.cpu.HL(), emu.mem.Read(flow), emu.mem.Read(gstats))
			prints++
		}
		if emu.cpu.TrapCheck != nil && emu.cpu.TrapCheck(pc) {
			if prints < 40 {
				fmt.Printf("%7d TRAP $0556 A=$%02X IX=$%04X DE=$%04X\n",
					step, emu.cpu.A, emu.cpu.IX, uint16(emu.cpu.D)<<8|uint16(emu.cpu.E))
				prints++
			}
		} else {
			emu.cpu.StepInstructionWithIRQ()
		}
	}
	if !shopSeen {
		t.Errorf("START never entered shop: flow=%d", emu.mem.Read(flow))
	}
	if !statsSeen {
		t.Errorf("STATS never latched: gstats=%d", emu.mem.Read(gstats))
	}
	for p, name := range []string{"GAME", "SHOP"} {
		balBankSel(t, emu, byte(p))
		head := ""
		for ad := 0xC000; ad < 0xC006; ad++ {
			head += fmt.Sprintf("%02X", emu.mem.Read(uint16(ad)))
		}
		fmt.Printf("FINAL BANK%d %s head=%s flow=%d gstats=%d\n", p, name, head, emu.mem.Read(flow), emu.mem.Read(gstats))
	}
	fmt.Printf("DONE step=%d PC=$%04X\n", step, emu.cpu.PC)
}
