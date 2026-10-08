package main

import (
	"fmt"
	"testing"
)

func TestBalatroEmitProbe(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	balPress(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	fmt.Printf("MODE=%d\n", balPeek(emu, syms["MODE"]))
	k := balatroKeys["6"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	armedAt := -1
	skE := syms["skEmit"]
	hist := map[uint16]int{}
	passes := 0
	for i := 0; i < 3000000 && passes < 4; i++ {
		if emu.cpu.PC == skE {
			fmt.Printf("skEmit A=%02X (EV) i=%d\n", emu.cpu.A, i)
			passes++
		}
		if armedAt < 0 && balPeek(emu, syms["REL"]) == 1 {
			armedAt = i
			emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
		}
		if i%300000 == 0 {
						line := ""
			for r := 0; r < 8; r++ {
				line += fmt.Sprintf("%02X ", balPeek(emu, syms["RBUF"]+uint16(r)))
			}
			fmt.Printf("tick%d PC=%04X REL=%d RBUF=%s\n", i/300000, emu.cpu.PC, balPeek(emu, syms["REL"]), line)
		}
		hist[emu.cpu.PC]++
		emu.cpu.StepInstructionWithIRQ()
	}
	top := 0
	var tp uint16
	for pc, n := range hist {
		if n > top {
			top, tp = n, pc
		}
	}
	fmt.Printf("toppc %04X x%d (skE=%04X)\n", tp, top, skE)
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	balRunFrames(emu, 2)
	fmt.Printf("CURSOR=%d SELX=%d\n", balPeek(emu, syms["CURSOR"]), balPeek(emu, syms["SELX"]))
}
