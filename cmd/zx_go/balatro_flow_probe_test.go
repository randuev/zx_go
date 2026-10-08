package main

import (
	"fmt"
	"testing"
)

func TestBalatroFlowProbe(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	st := func(tag string) {
		fmt.Printf("%-14s PC=%04X MODE=%d NSEL=%d CURSOR=%d DISCS=%d HANDS=%d HAND[0]=%d\n",
			tag, emu.cpu.PC, balPeek(emu, syms["MODE"]), balPeek(emu, syms["NSEL"]),
			balPeek(emu, syms["CURSOR"]), balPeek(emu, syms["DISCS"]), balPeek(emu, syms["HANDS"]),
			balPeek(emu, syms["HAND"]))
	}
	st("boot")
	pressOnce(t, emu, syms, "SPACE")
	st("afterSTART")
	pressOnce(t, emu, syms, "ENTER")
	st("sel0")
	pressOnce(t, emu, syms, "6")
	st("cur1")
	pressOnce(t, emu, syms, "ENTER")
	st("sel1")
	pressOnce(t, emu, syms, "D")
	st("afterD")
	balRunFrames(emu, 2)
	st("d+frames")
	// four-kind play
	for i := 0; i < 4; i++ {
		emu.mem.Write(syms["HAND"]+uint16(i), byte(i*16+12))
	}
	for i := 0; i < 5; i++ {
		pressOnce(t, emu, syms, "ENTER")
		pressOnce(t, emu, syms, "6")
	}
	st("sel5")
	pressOnce(t, emu, syms, "SPACE")
	st("afterPLAY")
	for f := 0; f < 10; f++ {
		balRunFrames(emu, 4)
		fmt.Printf("  score f=%d MODE=%d HTIDX=%d TOTlo=%d WINF=%d\n",
			f, balPeek(emu, syms["MODE"]), balPeek(emu, syms["HTIDX"]),
			balPeek(emu, syms["TOT"]), balPeek(emu, syms["WINF"]))
	}
}
