package main

import (
	"fmt"
	"testing"
)

// direct call bcdAdd1 with CHIPS=000093, A=$11 -> expect 000104
func TestBalatroDis8A(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	emu.mem.Write(syms["CHIPS"], 0x00)
	emu.mem.Write(syms["CHIPS"]+1, 0x00)
	emu.mem.Write(syms["CHIPS"]+2, 0x93)
	ret := uint16(0xBF00)
	emu.mem.Write(ret, 0x76) // halt pad
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(ret&0xFF))
	emu.mem.Write(emu.cpu.SP+1, byte(ret>>8))
	emu.cpu.A = 0x11
	emu.cpu.PC = syms["bcdAdd1"]
	for i := 0; i < 200; i++ {
		pc := emu.cpu.PC
		op := balPeek(emu, pc)
		emu.cpu.StepInstruction()
		fmt.Printf("pc=%04X op=%02X A=%02X F=%02X CHIPS=%02X%02X%02X\n",
			pc, op, emu.cpu.A, emu.cpu.F,
			balPeek(emu, syms["CHIPS"]), balPeek(emu, syms["CHIPS"]+1), balPeek(emu, syms["CHIPS"]+2))
		if emu.cpu.PC == ret {
			break
		}
	}
}
