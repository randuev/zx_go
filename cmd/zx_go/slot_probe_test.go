package main

import (
	"fmt"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSlotProbe: where do $C000/$4000 CPU writes land for classic +2
// paging values? Writes $AA at $C000 and $55 at $4000 per setting, dumps
// physical bank chips.
func TestSlotProbe(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 10; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for _, pgVal := range []byte{0x07, 0x05, 0x0D, 0x0F, 0x00, 0x02} {
		// program at $8010: LD A,val / LD HL,$8000 / LD BC,$7FFD / OUTI / HALT
		prog := []byte{0x3E, pgVal, 0x21, 0x00, 0x80, 0x01, 0xFD, 0x7F, 0xED, 0x79, 0x76}
		for i, b := range prog {
			emu.mem.Write(0x8010+uint16(i), b)
		}
		emu.cpu.PC = 0x8010
		for i := 0; i < 6; i++ {
			emu.cpu.StepInstructionWithIRQ()
			if emu.cpu.PC == 0x801A {
				break
			}
		}
		// now write markers via CPU $C000 / $4000
		prog2 := []byte{0x3E, 0xAA, 0x32, 0x00, 0xC0, 0x3E, 0x55, 0x32, 0x00, 0x40, 0x76}
		for i, b := range prog2 {
			emu.mem.Write(0x8020+uint16(i), b)
		}
		emu.cpu.PC = 0x8020
		for i := 0; i < 6; i++ {
			emu.cpu.StepInstructionWithIRQ()
			if emu.cpu.PC == 0x802A {
				break
			}
		}
		// dump first byte of every 8K physical chip 8..15
		out := ""
		for c := 8; c < 16; c++ {
			p := emu.mem.RAM8KPage(c)
			out += fmt.Sprintf("chip%d=%02X ", c, p[0])
		}
		fmt.Printf("pg=$%02X -> %s\n", pgVal, out)
	}
}
