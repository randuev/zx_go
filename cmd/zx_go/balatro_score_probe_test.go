package main

import (
	"fmt"
	"testing"
)

func callScore(t *testing.T, emu *emulator, target uint16) {
	t.Helper()
	ret := uint16(0xBF00)
	emu.mem.Write(ret, 0x76) // halt pad
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(ret&0xFF))
	emu.mem.Write(emu.cpu.SP+1, byte(ret>>8))
	emu.cpu.PC = target
	for i := 0; i < 400000; i++ {
		emu.cpu.StepInstruction()
		if emu.cpu.PC == ret {
			return
		}
	}
	t.Fatalf("callScore timeout PC=%04X SP=%04X", emu.cpu.PC, emu.cpu.SP)
}

// direct computeScore: hand code rank*4+suit. Four aces(12)+2s kicker.
func TestBalatroScoreProbe(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	// PLAYED = 4 aces + kicker
	played := []byte{0*16+12, 1*16+12, 2*16+12, 3*16+12, 2*16+2}
	for i, v := range played {
		emu.mem.Write(syms["PLAYED"]+uint16(i), v)
	}
	emu.mem.Write(syms["NPLAY"], 5)
	emu.mem.Write(syms["JOKSLOTS"], 0)
	callScore(t, emu, syms["computeScore"])
	fmt.Printf("4AKHTIDX=%d CHIPS=%d MULT=%d TOT=%d WINF=%d\n",
		balPeek(emu, syms["HTIDX"]), balBCD3(emu, syms["CHIPS"]),
		balPeek(emu, syms["MULTBIN"]), balBCD3(emu, syms["TOT"]), balPeek(emu, syms["WINF"]))

	// pair of aces
	p2 := []byte{0*16+12, 3*16+12, 1*16+5, 2*16+9, 0*16+1}
	for i, v := range p2 {
		emu.mem.Write(syms["PLAYED"]+uint16(i), v)
	}
	callScore(t, emu, syms["computeScore"])
	fmt.Printf("PAIR HTIDX=%d CHIPS=%d MULT=%d TOT=%d\n",
		balPeek(emu, syms["HTIDX"]), balBCD3(emu, syms["CHIPS"]),
		balPeek(emu, syms["MULTBIN"]), balBCD3(emu, syms["TOT"]))

	// high card
	hc := []byte{0, 1, 2, 3, 4}
	for i, v := range hc {
		emu.mem.Write(syms["PLAYED"]+uint16(i), v)
	}
	callScore(t, emu, syms["computeScore"])
	fmt.Printf("HIGH HTIDX=%d CHIPS=%d MULT=%d TOT=%d\n",
		balPeek(emu, syms["HTIDX"]), balBCD3(emu, syms["CHIPS"]),
		balPeek(emu, syms["MULTBIN"]), balBCD3(emu, syms["TOT"]))
}
