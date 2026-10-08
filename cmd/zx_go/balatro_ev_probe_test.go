package main

import (
	"fmt"
	"testing"
)

func TestBalatroEvProbe(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	sc := syms["scanKey"]
	hits := 0
	emu.cpu.AddPreFetchHook("sc", func(pc uint16) {
		if pc == sc {
			hits++
		}
	})
	region := map[uint16]int{}
	for i := 0; i < 200000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		region[emu.cpu.PC>>8]++
	}
	fmt.Printf("scanHits=%d regions: ", hits)
	for r := 0; r < 16; r++ {
		n := region[uint16(r)]
		if n > 0 {
			fmt.Printf("$%X:%d ", r, n)
		}
	}
	fmt.Printf("\nSP=%04X PC=%04X REL=%d\n", emu.cpu.SP, emu.cpu.PC, balPeek(emu, syms["REL"]))
}
