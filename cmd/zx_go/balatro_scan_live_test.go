package main

import (
	"fmt"
	"testing"
)

func TestBalatroScanLive(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 4)
	balPress(t, emu, syms, "SPACE")
	balRunFrames(emu, 8)
	fmt.Printf("MODE=%d REL=%d PC=%04X\n", balPeek(emu, syms["MODE"]), balPeek(emu, syms["REL"]), emu.cpu.PC)
	k := balatroKeys["6"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	sk := syms["scanKey"]
	seen := 0
	for i := 0; i < 3000000 && seen < 6; i++ {
		if emu.cpu.PC == sk {
			seen++
			sp := emu.cpu.SP
			lo := balPeek(emu, sp)
			hi := balPeek(emu, sp+1)
			// run to return
			for j := 0; j < 3000; j++ {
				emu.cpu.StepInstruction()
				if uint16(lo)|uint16(hi)<<8 == emu.cpu.PC {
					break
				}
			}
			fmt.Printf("scan#%d retA=%02X REL=%d\n", seen, emu.cpu.A, balPeek(emu, syms["REL"]))
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	fmt.Printf("scans seen=%d CURSOR=%d\n", seen, balPeek(emu, syms["CURSOR"]))
}
