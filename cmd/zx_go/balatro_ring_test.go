package main

import (
	"fmt"
	"testing"
)

// full play sequence; track TOT evolution through score animation
func TestBalatroRing(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	for i := 0; i < 4; i++ {
		emu.mem.Write(syms["HAND"]+uint16(i), byte(i*16+12))
	}
	emu.mem.Write(syms["HAND"]+4, byte(2*16+0))
	for i := 0; i < 5; i++ {
		pressOnce(t, emu, syms, "ENTER")
		pressOnce(t, emu, syms, "6")
	}
	// SPACE play + log each bcdAddTot pass
	k := balatroKeys["SPACE"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	prevC := balPeek(emu, syms["CHIPS"]+2)
	bat := syms["bcdAddTot"]
	for i := 0; i < 800000; i++ {
		pc := emu.cpu.PC
		emu.cpu.StepInstructionWithIRQ()
		if pc == bat+0x14 {
			fmt.Printf("addPass chips=%02X%02X%02X TOT=%02X%02X%02X MLOOP=%d\n",
				balPeek(emu, syms["CHIPS"]), balPeek(emu, syms["CHIPS"]+1), balPeek(emu, syms["CHIPS"]+2),
				balPeek(emu, syms["TOT"]), balPeek(emu, syms["TOT"]+1), balPeek(emu, syms["TOT"]+2),
				balPeek(emu, syms["MLOOP"]))
		}
		if pc == syms["csMulE"] && prevC != balPeek(emu, syms["CHIPS"]+2) {
			prevC = balPeek(emu, syms["CHIPS"]+2)
		}
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	prevTot := -1
	prevMode := -1
	for i := 0; i < 60; i++ {
		balRunFrames(emu, 1)
		tt := balBCD3(emu, syms["TOT"])
		m := int(balPeek(emu, syms["MODE"]))
		if tt != prevTot || m != prevMode {
			fmt.Printf("f%d TOT=%d SCOREV=%d MODE=%d SCPH=%d\n", i, tt,
				balBCD3(emu, syms["SCOREV"]), m, balPeek(emu, syms["SCPH"]))
			prevTot = tt
			prevMode = m
		}
	}
	fmt.Printf("end TOT=%d SCOREV=%d MODE=%d\n", balBCD3(emu, syms["TOT"]), balBCD3(emu, syms["SCOREV"]), balPeek(emu, syms["MODE"]))
}
