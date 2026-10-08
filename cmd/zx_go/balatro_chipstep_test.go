package main

import (
	"fmt"
	"testing"
)

// trace the first 40 instructions of the csC card-scoring loop (step by step)
func TestBalatroChipStep(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	played := []byte{0*16 + 12, 1*16 + 12, 2*16 + 12, 3*16 + 12, 2*16 + 2}
	for i, v := range played {
		emu.mem.Write(syms["PLAYED"]+uint16(i), v)
	}
	emu.mem.Write(syms["NPLAY"], 5)
	emu.mem.Write(syms["JOKSLOTS"], 0)
	ret := uint16(0xBF00)
	emu.mem.Write(ret, 0x76) // halt pad
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(ret&0xFF))
	emu.mem.Write(emu.cpu.SP+1, byte(ret>>8))
	csC := syms["csC"]
	emu.cpu.PC = syms["computeScore"]
	// fast-forward until first csC entry
	for i := 0; i < 300000; i++ {
		if emu.cpu.PC == csC {
			break
		}
		emu.cpu.StepInstruction()
	}
	// now single-step and print each instruction's PC + relevant regs
	for i := 0; i < 48; i++ {
		fmt.Printf("pc=%04X HL=%04X DE=%04X A=%02X chips=%02X%02X%02X b=%d\n",
			emu.cpu.PC, emu.cpu.HL(), emu.cpu.DE(), emu.cpu.A,
			balPeek(emu, syms["CHIPS"]), balPeek(emu, syms["CHIPS"]+1), balPeek(emu, syms["CHIPS"]+2),
			balPeek(emu, syms["CIX"]))
		emu.cpu.StepInstruction()
		if emu.cpu.PC == ret {
			break
		}
	}
}
