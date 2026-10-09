package main

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatroKeySanity(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	for i := 0; i < 60; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("pre REL=%02X MODE=%02X", balPeek(emu, syms["REL"]), balPeek(emu, syms["MODE"]))
	emu.kbd.PressMatrixKey(6, 0x01, true)
	seen := false
	for i := 0; i < 3000000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		if balPeek(emu, syms["REL"]) == 1 {
			t.Logf("armed after %d insns PC=%04X MODE=%02X", i, emu.cpu.PC, balPeek(emu, syms["MODE"]))
			seen = true
			break
		}
	}
	if !seen {
		var rb [8]int
		for r := 0; r < 8; r++ {
			rb[r] = int(balPeek(emu, syms["RBUF"]+uint16(r)))
		}
		t.Logf("never armed RBUF=% X REL=%02X PC=%04X MODE=%02X", rb, balPeek(emu, syms["REL"]), emu.cpu.PC, balPeek(emu, syms["MODE"]))
	}
}
