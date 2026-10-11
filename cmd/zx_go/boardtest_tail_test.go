package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardtestTailSpy : bitmap columns 32..39 (x256..319) across all bands.
func TestBoardtestTailSpy(t *testing.T) {
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
	for i := 0; i < 3_000_000; i++ {
		if balPeek(emu, emu.cpu.PC) == 0x76 {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	pb := func(y int) uint16 {
		return 0x4000 + uint16((y&7)<<8) + uint16((y&0x38)<<2) + uint16((y&0xC0)<<5)
	}
	// physical page $4000 third 0 = $4000..$57FF ; third covers y0..191 all.
	// columns 32..39 byte addrs : third groups $4000,$4800,$5000 char-row addrs ($4020...$47E0)*... build explicitly:
	fmt.Println("== columns 32-39 (x256-319) band map ==")
	lit := 0
	for band := 0; band < 24; band++ {
		any := ""
		for py := 0; py < 8; py++ {
			y := band*8 + py
			line := ""
			for c := 32; c < 40; c++ {
				v := balPeek(emu, pb(y)+uint16(c))
				if v != 0 {
					line += "#"
					lit++
				} else {
					line += "."
				}
			}
			if line != "........" {
				any += fmt.Sprintf(" y%d %s", y, line)
			}
		}
		if any != "" {
			fmt.Printf("b%-2d%s\n", band, any)
		}
	}
	fmt.Println("lit tail bytes:", lit)
}

// TestBoardtestAttrTail : attrs rows 22-23 all cols.
func TestBoardtestAttrTail(t *testing.T) {
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
	for i := 0; i < 3_000_000; i++ {
		if balPeek(emu, emu.cpu.PC) == 0x76 {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	for r := 22; r <= 23; r++ {
		line := ""
		for c := 0; c < 40; c++ {
			line += fmt.Sprintf("%02X ", balPeek(emu, 0x5800+uint16(r*32+c)))
		}
		fmt.Printf("a%d %s\n", r, line)
	}
}
