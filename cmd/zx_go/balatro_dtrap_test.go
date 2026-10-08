package main

import (
	"fmt"
	"testing"
)

func TestBalatroDTrap(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "SPACE")
	pressOnce(t, emu, syms, "ENTER")
	pressOnce(t, emu, syms, "6")
	pressOnce(t, emu, syms, "ENTER")
	// press D, keep pressed, catch first escape
	k := balatroKeys["D"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	lastIn := uint16(0xFFFF)
	trapped := false
	var b [8]byte
	var stk [10]byte
	var ring [40]uint16
	ri := 0
	for i := 0; i < 2_000_000 && !trapped; i++ {
		emu.cpu.StepInstructionWithIRQ()
		pc := emu.cpu.PC
		ring[ri%40] = pc
		ri++
		if pc >= 0x8000 && pc <= 0xBFFF {
			lastIn = pc
		} else if lastIn != 0xFFFF {
			for j := 0; j < 8; j++ {
				if lastIn+uint16(j) >= 0x8000 {
					b[j] = balPeek(emu, lastIn+uint16(j))
				}
			}
			for j := 0; j < 10; j++ {
				stk[j] = balPeek(emu, emu.cpu.SP+uint16(j))
			}
			trapped = true
		}
	}
	if !trapped {
		t.Logf("no escape (good) D handled: PC=%04X", emu.cpu.PC)
		return
	}
	fmt.Printf("escaped PC=%04X lastIn=$%04X bytes=%02X\n", emu.cpu.PC, lastIn, b)
	fmt.Printf("SP=%04X stk=%02X\n", emu.cpu.SP, stk)
	fmt.Printf("ring tail:")
	for j := 0; j < 40; j++ {
		fmt.Printf(" %04X", ring[(ri-40+j)%40])
	}
	fmt.Printf("\n")
}
