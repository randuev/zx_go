package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBoardtestRowtab(t *testing.T) {
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
	for i, v := range bin {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	pb := func(y int) uint16 {
		return 0x4000 + uint16((y&7)<<8) + uint16((y&0x38)<<2) + uint16((y&0xC0)<<5)
	}
	bad := 0
	for y := 0; y < 192; y++ {
		a := uint16(0x8A85 + y*2)
		got := uint16(balPeek(emu, a)) | uint16(balPeek(emu, a+1))<<8
		if got != pb(y) {
			fmt.Printf("ROWTAB[%d]=$%04X expected $%04X\n", y, got, pb(y))
			bad++
		}
	}
	fmt.Println("bad:", bad)
}
