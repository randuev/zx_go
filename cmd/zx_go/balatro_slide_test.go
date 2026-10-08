package main

import (
	"fmt"
	"testing"
)

// TestBalatroSlideTrap catches the first PC that lands in the data region
// (>= DATAstart) and prints the last CODE-range PC + its bytes + stack,
// to locate the jp/call/ret that jumps into data.
func TestBalatroSlideTrap(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	end := syms["ENDMARK"]
	balRunFrames(emu, 6)
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	dataStart := uint16(0x9600)
	lastCode := uint16(0)
	var lastCodeBytes [6]byte
	trapped := false
	for i := 0; i < 1_600_000 && !trapped; i++ {
		emu.cpu.StepInstructionWithIRQ()
		pc := emu.cpu.PC
		if pc < dataStart {
			lastCode = pc
		} else if lastCode != 0 {
			for j := 0; j < 6; j++ {
				lastCodeBytes[j] = balPeek(emu, lastCode+uint16(j))
			}
			trapped = true
		}
	}
	if !trapped {
		t.Logf("no slide (good); end=%04X", end)
		return
	}
	fmt.Printf("entered data@%04X end=%04X | lastCode=$%04X bytes=%v\n",
		emu.cpu.PC, end, lastCode, lastCodeBytes)
	fmt.Printf("SP=%04X stk=%02X%02X %02X%02X MODE=%d NSEL=%d CURSOR=%d\n",
		emu.cpu.SP, balPeek(emu, emu.cpu.SP+1), balPeek(emu, emu.cpu.SP),
		balPeek(emu, emu.cpu.SP+3), balPeek(emu, emu.cpu.SP+2),
		balPeek(emu, syms["MODE"]), balPeek(emu, syms["NSEL"]), balPeek(emu, syms["CURSOR"]))
}
