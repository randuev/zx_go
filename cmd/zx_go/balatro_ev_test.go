package main

import (
	"fmt"
	"testing"
)

// TestBalatroEvTrace follows the ENTER-deal path: it records the PC ring so
// the first wandering region after beginGame can be disassembled precisely.
func TestBalatroEvTrace(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	ring := make([]uint16, 0, 300000)
	escape := false
	for f := 0; f < 250000; f++ {
		emu.cpu.StepInstructionWithIRQ()
		ring = append(ring, emu.cpu.PC)
		if !escape && (emu.cpu.PC < 0x8000 || emu.cpu.PC > 0xBFFF) {
			escape = true
			lo := len(ring) - 400
			if lo < 0 {
				lo = 0
			}
			fmt.Printf("ESCAPE at insn %d, prev PCs:\n", len(ring))
			for _, p := range ring[lo:] {
				fmt.Printf("%04X ", p)
			}
			fmt.Printf("\nSP=%04X\n", emu.cpu.SP)
			break
		}
	}
	if !escape {
		fmt.Printf("no escape; last PC=%04X SP=%04X\n", emu.cpu.PC, emu.cpu.SP)
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	fmt.Printf("last pcs: ")
	for _, p := range ring[len(ring)-40:] {
		fmt.Printf("%04X ", p)
	}
	fmt.Printf("\nSP=%04X PC=%04X\n", emu.cpu.SP, emu.cpu.PC)
}

func TestBalatroEscapeWhen(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	ml := 0
	ins := 0
	for f := 0; f < 8; f++ {
		for s := 0; s < 43000; s++ {
			emu.cpu.StepInstructionWithIRQ()
			ins++
			if emu.cpu.PC == syms["mainLoop"] {
				ml++
			}
			if emu.cpu.PC < 0x8000 || emu.cpu.PC > 0xBFFF {
				fmt.Printf("ESCAPE frame %d insn %d pc=%04X mainLoop=%d\n", f, ins, emu.cpu.PC, ml)
				return
			}
		}
	}
	fmt.Printf("NO ESCAPE, mainLoop=%d\n", ml)
}
