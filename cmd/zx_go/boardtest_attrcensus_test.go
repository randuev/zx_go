package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardtestAttrCensus : raw attr census of the v5.3.5 board after direct-load run.
// Prints attr bytes for sidebar rows and playfield rows so paper colors are
// verifiable against the mock without palette-interpretation guesswork.
func TestBoardtestAttrCensus(t *testing.T) {
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
	fmt.Printf("done pc=$%04X\n", emu.cpu.PC)
	// attr bytes b0..b23, cols 0..39
	for b := 0; b < 24; b++ {
		line := ""
		for c := 0; c < 32; c++ {
			line += fmt.Sprintf("%02X ", balPeek(emu, 0x5800+uint16(b*32+c)))
		}
		fmt.Printf("a%02d %s\n", b, line)
	}
	// paper-color breakdown per band
	name := func(p int) string {
		return []string{"blk", "blu", "red", "mag", "grn", "cyn", "yel", "wht"}[p&7]
	}
	for b := 0; b < 24; b++ {
		counts := map[string]int{}
		for c := 0; c < 32; c++ {
			p := int(balPeek(emu, 0x5800+uint16(b*32+c))>>3) & 7
			counts[name(p)]++
		}
		fmt.Printf("b%02d paper:", b)
		for k, v := range counts {
			fmt.Printf(" %s=%d", k, v)
		}
		fmt.Println()
	}
}
