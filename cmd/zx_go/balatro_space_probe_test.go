package main

import (
	"fmt"
	"testing"
)

func TestBalatroSpaceProbe(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	sk := syms["scanKey"]
	rel := syms["REL"]
	k := balatroKeys["SPACE"]
	fmt.Printf("start PC=%04X IFF1=%v IFF2=%v MODE=%d REL=%d sk=%04X\n",
		emu.cpu.PC, emu.cpu.IFF1, emu.cpu.IFF2,
		balPeek(emu, syms["MODE"]), balPeek(emu, rel), sk)
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	scanned := false
	for i := 0; i < 60000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		if emu.cpu.PC == sk {
			scanned = true
		}
		if i%2000 == 0 || (scanned && balPeek(emu, rel) == 1) {
			fmt.Printf("i=%d PC=%04X REL=%d scanned=%v IFF1=%v\n",
				i, emu.cpu.PC, balPeek(emu, rel), scanned, emu.cpu.IFF1)
		}
		if scanned && balPeek(emu, rel) == 1 {
			break
		}
	}
	fmt.Printf("FINAL scanned=%v REL=%d PC=%04X MODE=%d\n", scanned, balPeek(emu, rel), emu.cpu.PC, balPeek(emu, syms["MODE"]))
}
