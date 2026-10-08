package main

import (
	"fmt"
	"testing"
)

func TestBalatroStepThroughSfx(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	for i := 0; i < 200000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		if emu.cpu.PC == syms["beginGame"] {
			break
		}
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	for i := 0; i < 40000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		if emu.cpu.PC == 0x805E {
			break
		}
	}
	fmt.Printf("at call: PC=%04X SP=%04X\n", emu.cpu.PC, emu.cpu.SP)
	for i := 0; i < 40; i++ {
		emu.cpu.StepInstructionWithIRQ()
		fmt.Printf("PC=%04X SP=%04X A=%02X top=%02X%02X\n", emu.cpu.PC, emu.cpu.SP, emu.cpu.A,
			emu.mem.Read(emu.cpu.SP+1), emu.mem.Read(emu.cpu.SP))
	}
}
