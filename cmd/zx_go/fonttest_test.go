package main

import (
	"fmt"
	"image/png"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestFontTest renders the STANDALONE 6px font unit (demos/balatro/unit) —
// proves putc6/drawstr glyph shapes and true 6px pitch before the engine
// inherits them. Ground truth = fonttest.png eyeballed vs mock6x8.
func TestFontTest(t *testing.T) {
	bin, err := os.ReadFile("/root/nerve-workspace/demos/balatro/unit/unit.bin")
	if err != nil {
		t.Fatal(err)
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bin {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000
	// trace every CALL/RET boundary crossing so stack balance is visible
	inCode := true
	for i := 0; i < 600000; i++ {
		pc := emu.cpu.PC
		opcode := balPeek(emu, pc)
		emu.cpu.StepInstruction()
		npc := emu.cpu.PC
		if opcode == 0xCD && npc >= 0x8000 && npc < 0x8200 {
			t.Logf("CALL $%04X from $%04X SP=$%04X", npc, pc, emu.cpu.SP)
		}
		if opcode == 0xC9 {
			t.Logf("RET from $%04X -> $%04X SP=$%04X A=$%02X BC=%04X DE=%04X", pc, npc, emu.cpu.SP, emu.cpu.A, emu.cpu.BC(), emu.cpu.DE())
		}
		if pc >= 0x8000 && npc < 0x8000 {
			stk := ""
			for j := 0; j < 8; j++ {
				stk += fmt.Sprintf("[%04X]=%02X ", emu.cpu.SP+uint16(j), balPeek(emu, emu.cpu.SP+uint16(j)))
			}
			t.Logf("LEFT CODE prevPC=$%04X -> pc=$%04X SP=$%04X %s", pc, npc, emu.cpu.SP, stk)
			break
		}
		_ = inCode
	}
	f, err := os.Create("/root/nerve-workspace/demos/balatro/proofs/fonttest.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, emu.renderFrame()); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	// ink census per text band: lit pixel count + leftmost/rightmost ink column
	for _, r := range []int{1, 3, 5, 7, 9, 11, 13, 15} {
		lit := 0
		first, last := -1, -1
		for y := r * 8; y < r*8+8; y++ {
			for x := 0; x < 256; x++ {
				a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3))
				b := balPeek(emu, a)
				if (b>>(7-(x&7)))&1 == 1 {
					lit++
					if first < 0 || x < first {
						first = x
					}
					if x > last {
						last = x
					}
				}
			}
		}
		fmt.Printf("fontrow b%02d lit=%d x=%d..%d\n", r, lit, first, last)
	}
				fmt.Printf("fonttest done PC=%04X\n", emu.cpu.PC)
}
