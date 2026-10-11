package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestAttChipDump(t *testing.T) {
	bin, err := os.ReadFile("/root/nerve-workspace/demos/balatro/boardtest.bin")
	if err != nil { t.Fatal(err) }
	prev := cliFlagsActive
	nf := cliFlags{}; nf.noSound = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil { t.Fatal(err) }
	emu.paused.Store(false)
	for i := 0; i < 220; i++ { runOneFrameHeadless(emu, roms.ModelPlus2) }
	for i, v := range bin { emu.mem.Write(0x8000+uint16(i), v) }
	emu.cpu.SP = 0xBF00
	emu.cpu.PC = 0x8000
	for i := 0; i < 3000000; i++ {
		if balPeek(emu, emu.cpu.PC) == 0x76 { break }
		emu.cpu.StepInstruction()
	}
	for b := 0; b < 2; b++ {
		line := ""
		for c := 8; c < 32; c++ { line += fmt.Sprintf("%02X ", balPeek(emu, uint16(0x5800+b*32+c))) }
		fmt.Printf("att b%d c8-31: %s\n", b, line)
	}
}
