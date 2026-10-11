package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBoardtestAttrTailSpy(t *testing.T) {
	bin, _ := os.ReadFile("/root/nerve-workspace/demos/balatro/boardtest.bin")
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, _ := newEmulator(roms.ModelPlus2)
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bin {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	emu.mem.Write(0xBF00, 0x76)
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFF00, 0x00)
	emu.mem.Write(0xFF01, 0xBF)
	emu.cpu.PC = 0x8000
	for a := 0x5AE0; a <= 0x5AFF; a++ {
		emu.mem.Write(uint16(a), 0)
	}
	seen := map[string]int{}
	for i := 0; i < 3_000_000; i++ {
		pc := emu.cpu.PC
		if balPeek(emu, pc) == 0x76 {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
		hl := emu.cpu.HL()
		if hl >= 0x5AE0 && hl <= 0x5AFF {
			v := balPeek(emu, hl)
			if v != 0 {
				k := fmt.Sprintf("pc=$%04X hl=$%04X A=$%02X", pc, hl, v)
				seen[k]++
			}
		}
	}
	for k, n := range seen {
		fmt.Printf("%s x%d\n", k, n)
	}
}
