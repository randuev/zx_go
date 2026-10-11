package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardtestCmdSpy : watch putc6 during bands 22-23 only. Log ascii-in,
// C=Xpix-in, BX-global-after, HL-out-of-pxHL.
func TestBoardtestCmdSpy(t *testing.T) {
	bin, err := os.ReadFile("/root/nerve-workspace/demos/balatro/boardtest.bin")
	if err != nil {
		t.Fatal(err)
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
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
	inBar := false
	// drawstr6 entry $8131? find via sym : just monitor putc6 entry $80E4 (PC check)
	cnt := 0
	for i := 0; i < 3_000_000; i++ {
		pc := emu.cpu.PC
		if balPeek(emu, pc) == 0x76 {
			break
		}
		if pc == 0x80E4 { // putc6 entry
			b := emu.cpu.B
			c := emu.cpu.C
			a := emu.cpu.A
			if b >= 22 {
				inBar = true
				if cnt < 40 {
					fmt.Printf("putc6 b=%d xpix=%d ch=%c BX=%d Y=%d\n", b, c, a, balPeek(emu, 0x88EF), balPeek(emu, 0x88EC))
					cnt++
				}
			}
		}
		if inBar && pc == 0x814E+2 { // right after first pxHL call in p6L? verify opcode
			// informational
		}
		emu.cpu.StepInstructionWithIRQ()
	}
}
