package main

import (
	"bufio"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardtestDump : render boardtest once, dump full 320x192 bitmap to PBM.
func TestBoardtestDump(t *testing.T) {
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
	for i := 0; i < 3_000_000; i++ {
		if balPeek(emu, emu.cpu.PC) == 0x76 {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	pb := func(y int) uint16 {
		return 0x4000 + uint16((y&7)<<8+(y&0x38)<<2+(y&0xC0)<<5)
	}
	f, _ := os.Create("/tmp/cur2.pbm")
	w := bufio.NewWriter(f)
	w.WriteString("P1\n320 192\n")
	for y := 0; y < 192; y++ {
		line := ""
		for c := 0; c < 40; c++ {
			v := balPeek(emu, pb(y)+uint16(c))
			for b := 7; b >= 0; b-- {
				if v&(1<<uint(b)) != 0 {
					line += "#"
				} else {
					line += "."
				}
			}
		}
		w.WriteString(line + "\n")
	}
	w.Flush()
	f.Close()
}
