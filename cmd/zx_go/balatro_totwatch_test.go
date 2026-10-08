package main

import (
	"fmt"
	"testing"
)

// Watch every write to TOT bytes (direct + indirect) with PC attribution.
func TestBalatroTotWatch(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	for i := 0; i < 4; i++ {
		emu.mem.Write(syms["HAND"]+uint16(i), byte(i*16+12))
	}
	emu.mem.Write(syms["HAND"]+4, byte(2*16+0))
	for i := 0; i < 5; i++ {
		pressOnce(t, emu, syms, "ENTER")
		pressOnce(t, emu, syms, "6")
	}
	k := balatroKeys["SPACE"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	tot := syms["TOT"]
	sv := syms["SCOREV"]
	prev := make([]byte, 6)
	for i := 0; i < 3; i++ {
		prev[i] = balPeek(emu, tot+uint16(i))
		prev[3+i] = balPeek(emu, sv+uint16(i))
	}
	var ring [40]uint16
	rj := 0
	for i := 0; i < 800000; i++ {
		pc := emu.cpu.PC
		ring[rj%40] = pc
		rj++
		emu.cpu.StepInstructionWithIRQ()
		for j := 0; j < 3; j++ {
			v := balPeek(emu, tot+uint16(j))
			if v != prev[j] {
				fmt.Printf("TOT+%d %02X->%02X pc=$%04X A=%02X HL=%04X DE=%04X SP=%04X\n",
					j, prev[j], v, emu.cpu.PC, emu.cpu.A, emu.cpu.HL(), emu.cpu.DE(), emu.cpu.SP)
				fmt.Printf("  ring:")
				for q := 0; q < 40; q++ {
					fmt.Printf("%04X ", ring[(rj-40+q)%40])
				}
				fmt.Printf("\n")
				prev[j] = v
			}
			v2 := balPeek(emu, sv+uint16(j))
			if v2 != prev[3+j] {
				fmt.Printf("SV+%d %02X->%02X pc=$%04X\n", j, prev[3+j], v2, emu.cpu.PC)
				prev[3+j] = v2
			}
		}
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	for i := 0; i < 80; i++ {
		balRunFrames(emu, 1)
		for j := 0; j < 3; j++ {
			v := balPeek(emu, tot+uint16(j))
			if v != prev[j] {
				fmt.Printf("f%d TOT+%d %02X->%02X\n", i, j, prev[j], v)
				prev[j] = v
			}
			v2 := balPeek(emu, sv+uint16(j))
			if v2 != prev[3+j] {
				fmt.Printf("f%d SV+%d %02X->%02X\n", i, j, prev[3+j], v2)
				prev[3+j] = v2
			}
		}
	}
	fmt.Printf("end TOT=%02X%02X%02X SV=%02X%02X%02X MODE=%d\n",
		balPeek(emu, tot), balPeek(emu, tot+1), balPeek(emu, tot+2),
		balPeek(emu, sv), balPeek(emu, sv+1), balPeek(emu, sv+2), balPeek(emu, syms["MODE"]))
}
