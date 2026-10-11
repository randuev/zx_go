package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardtestGhostSpy : attribute every write into bitmap byte-cols 32-39
// bands b22-b23 (the ghost text right of the cmd bar Seva saw).
func TestBoardtestGhostSpy(t *testing.T) {
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
	pb := func(y int) uint16 {
		return 0x4000 + uint16((y&7)<<8+(y&0x38)<<2+(y&0xC0)<<5)
	}
	// watch region : y176..191 cols 32..39
	watch := []uint16{}
	for y := 176; y <= 191; y++ {
		for c := 32; c <= 39; c++ {
			watch = append(watch, pb(y)+uint16(c))
		}
	}
	for _, a := range watch {
		emu.mem.Write(a, 0)
	}
	last := map[uint16]byte{}
	for _, a := range watch {
		last[a] = 0
	}
	hits := map[string]int{}
	for i := 0; i < 3_000_000; i++ {
		pc := emu.cpu.PC
		if balPeek(emu, pc) == 0x76 {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
		for _, a := range watch {
			v := balPeek(emu, a)
			if v != 0 && last[a] != v {
				key := fmt.Sprintf("pc=$%04X", pc)
				hits[key]++
				if hits[key] <= 6 {
					yv := int(balPeek(emu, 0x88FA))
					rt := uint16(balPeek(emu, 0x8A85+uint16(yv*2))) | uint16(balPeek(emu, 0x8A85+uint16(yv*2)+1))<<8
					fmt.Printf("HIT pc=$%04X hl=$%04X A=$%02X @addr=$%04X BX=%d Y=%d ROWTAB[Y]=$%04X C=%d iy=$%04X\n", pc, emu.cpu.HL(), emu.cpu.A, a, balPeek(emu, 0x88FD), yv, rt, emu.cpu.C, emu.cpu.IY)
				}
				last[a] = v
			}
		}
	}
	for k, n := range hits {
		fmt.Printf("%s  x%d\n", k, n)
	}
}
