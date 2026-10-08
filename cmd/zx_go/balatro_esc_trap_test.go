package main

import (
	"fmt"
	"testing"
)

// TestBalatroEscTrap holds ENTER, steps until PC leaves $8000-$BFFF, then
// dumps the last in-window PC, the bytes there, and the top of stack.
func TestBalatroEscTrap(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	lastIn := uint16(0xFFFF)
	trapped := false
	var lastBytes [6]byte
	var stk [8]uint8
	var hand [8]byte
	for i := 0; i < 1_600_000 && !trapped; i++ {
		emu.cpu.StepInstructionWithIRQ()
		pc := emu.cpu.PC
		if pc >= 0x8000 && pc <= 0xBFFF {
			lastIn = pc
		} else if lastIn != 0xFFFF {
			for j := 0; j < 6; j++ {
				if lastIn+uint16(j) >= 0x8000 {
					lastBytes[j] = balPeek(emu, lastIn+uint16(j))
				}
			}
			for j := 0; j < 8; j++ {
				stk[j] = balPeek(emu, emu.cpu.SP+uint16(j))
			}
			for j := 0; j < 8; j++ {
				hand[j] = balPeek(emu, syms["HAND"]+uint16(j))
			}
			trapped = true
		}
	}
	if !trapped {
		t.Logf("no escape in window (good); PC=%04X", emu.cpu.PC)
		return
	}
	fmt.Printf("escaped to PC=%04X\nlastIn=$%04X bytes=%v\nSP=%04X stk=%v MODE=%d CURSOR=%d NSEL=%d HAND[0..7]=%v\n",
		emu.cpu.PC, lastIn, lastBytes, emu.cpu.SP, stk[:8],
		balPeek(emu, syms["MODE"]), balPeek(emu, syms["CURSOR"]), balPeek(emu, syms["NSEL"]),
		hand)
}
