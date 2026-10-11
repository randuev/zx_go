package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardtestChipTrace : step-exec boardtest; when PC hits a chip
// 'call blit8' site, log entry regs, every bl8 store (HL,A), and the
// pxHL HL entering bl8.  Sites : $86C4 (b2 chip) and $86F9 (b5 chip).
func TestBoardtestChipTrace(t *testing.T) {
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
	trace := false
	for i := 0; i < 3_000_000; i++ {
		pc := emu.cpu.PC
		if pc == 0x8029 { // halt1 pad
			break
		}
		if pc == 0x86C4 || pc == 0x86F9 {
			trace = true
			fmt.Printf("ENTER chip blit8 @pc=$%04X  A=$%02X  BC=$%04X DE=$%04X HL=$%04X IX=$%04X  SPR=$%04X BAND=%d BX=%d Y=%d\n",
				pc, emu.cpu.A, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL(), emu.cpu.IX,
				uint16(balPeek(emu, 0x88F2))|uint16(balPeek(emu, 0x88F3))<<8,
				balPeek(emu, 0x88F6), balPeek(emu, 0x88EF), balPeek(emu, 0x88EC))
		}
		if trace {
			// log pxHL exit (HL set) and bl8 stores
			if pc == 0x81B7 {
				fmt.Printf("  pxHL out HL=$%04X\n", emu.cpu.HL())
			}
			if pc == 0x81BB {
				fmt.Printf("  store HL=$%04X A=$%02X  old=$%02X\n", emu.cpu.HL(), emu.cpu.A, balPeek(emu, emu.cpu.HL()))
			}
			if pc == 0x81C0 {
				fmt.Printf("  blit8 exit\n")
				trace = false
			}
		}
		emu.cpu.StepInstructionWithIRQ()
	}
}
