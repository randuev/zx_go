package main

import (
	"fmt"
	"testing"
)

// direct computeScore (four aces + kicker), log every MULTBIN/CHIPS write
func TestBalatroMultWatch(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	played := []byte{0*16 + 12, 1*16 + 12, 2*16 + 12, 3*16 + 12, 2*16 + 2}
	for i, v := range played {
		emu.mem.Write(syms["PLAYED"]+uint16(i), v)
	}
	emu.mem.Write(syms["NPLAY"], 5)
	emu.mem.Write(syms["JOKSLOTS"], 0)
	watch := []struct {
		a    uint16
		name string
	}{
		{syms["MULTBIN"], "MULT"}, {syms["STEPV"], "STEPV"}, {syms["CHIPSH"], "CHSH"}, {syms["CHIPSL"], "CHSL"},
		{syms["CHIPS"], "C0"}, {syms["CHIPS"] + 1, "C1"}, {syms["CHIPS"] + 2, "C2"},
		{syms["TOT"], "T0"}, {syms["TOT"] + 1, "T1"}, {syms["TOT"] + 2, "T2"},
	}
	prev := make([]byte, len(watch))
	for i, w := range watch {
		prev[i] = balPeek(emu, w.a)
	}
	ret := uint16(0xBF00)
	emu.mem.Write(ret, 0x76) // halt pad
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(ret&0xFF))
	emu.mem.Write(emu.cpu.SP+1, byte(ret>>8))
	emu.cpu.PC = syms["computeScore"]
	done := false
	for i := 0; i < 300000 && !done; i++ {
		emu.cpu.StepInstruction()
		for j, w := range watch {
			v := balPeek(emu, w.a)
			if v != prev[j] {
				fmt.Printf("%s: %02X->%02X pc=$%04X A=%02X HL=%04X DE=%04X\n",
					w.name, prev[j], v, emu.cpu.PC, emu.cpu.A, emu.cpu.HL(), emu.cpu.DE())
				prev[j] = v
			}
		}
		if emu.cpu.PC == ret {
			done = true
		}
	}
	fmt.Printf("final MULT=%02X CHIPS=%02X%02X%02X HTIDX=%d TOT=%02X%02X%02X WINF=%d\n",
		balPeek(emu, syms["MULTBIN"]), balPeek(emu, syms["CHIPS"]), balPeek(emu, syms["CHIPS"]+1),
		balPeek(emu, syms["CHIPS"]+2), balPeek(emu, syms["HTIDX"]), balPeek(emu, syms["TOT"]),
		balPeek(emu, syms["TOT"]+1), balPeek(emu, syms["TOT"]+2), balPeek(emu, syms["WINF"]))
}
