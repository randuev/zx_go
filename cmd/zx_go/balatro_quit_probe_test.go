package main

import (
	"fmt"
	"testing"
)

func TestBalatroQuitProbe(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	fmt.Printf("afterSPACE PC=%04X MODE=%d REL=%d SP=%04X\n", emu.cpu.PC, balPeek(emu, syms["MODE"]), balPeek(emu, syms["REL"]), emu.cpu.SP)
	k := balatroKeys["Q"]
	rel := syms["REL"]
	sk := syms["scanKey"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	scanned := false
	armed := false
	quitSeen := false
	fmt.Printf("IFF1=%v IM=%v\n", emu.cpu.IFF1, emu.cpu.IM)
	for i := 0; i < 400000; i++ {
		pc := emu.cpu.PC
		op := balPeek(emu, pc)
		if i < 40 {
			fmt.Printf("t%02d pc=$%04X op=%02X\n", i, pc, op)
		}
		emu.cpu.StepInstructionWithIRQ()
		np := emu.cpu.PC
		if pc >= syms["start"] && pc < syms["ENDMARK"] && (np < syms["start"] || np >= syms["ENDMARK"]) {
			fmt.Printf("XFER pc=$%04X op=%02X -> $%04X SP=%04X stk=$%02X%02X\n",
				pc, op, np, emu.cpu.SP, balPeek(emu, emu.cpu.SP+1), balPeek(emu, emu.cpu.SP))
			quitSeen = true
		}
		if quitSeen {
			fmt.Printf("post pc=$%04X IFF1=%v IM=%d\n", np, emu.cpu.IFF1, emu.cpu.IM)
			if np == 0xBF00 {
				break
			}
		}
		if np == sk {
			scanned = true
		}
		if scanned && balPeek(emu, rel) == 1 {
			armed = true
			break
		}
	}
	fmt.Printf("scanned=%v armed=%v PC=%04X REL=%d MODE=%d\n", scanned, armed, emu.cpu.PC, balPeek(emu, rel), balPeek(emu, syms["MODE"]))
}
