package main

import (
	"fmt"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocIFFProbe(t *testing.T) {
	if t.Name() != "TestBalatrocIFFProbe" {
		t.Skip()
	}
	emu := balBootLive(t)
	runF := func(n int) {
		for j := 0; j < n; j++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	runF(160)
	q := func() uint32 {
		a := balSym("qctr")
		return uint32(emu.mem.Read(a)) | uint32(emu.mem.Read(a+1))<<8 |
			uint32(emu.mem.Read(a+2))<<16 | uint32(emu.mem.Read(a+3))<<24
	}
	fmt.Printf("TITLE qctr=%d IFF1=%v IM=%d PC=%04X\n", q(), emu.cpu.IFF1, emu.cpu.IM, emu.cpu.PC)
	balStroke(emu, runF, 4, 0x01, 8)
	fmt.Printf("POST-START qctr=%d IFF1=%v IM=%d PC=%04X\n", q(), emu.cpu.IFF1, emu.cpu.IM, emu.cpu.PC)
	runF(20)
	fmt.Printf("+20f qctr=%d IFF1=%v IM=%d PC=%04X\n", q(), emu.cpu.IFF1, emu.cpu.IM, emu.cpu.PC)
	runF(60)
	fmt.Printf("+80f qctr=%d IFF1=%v IM=%d PC=%04X\n", q(), emu.cpu.IFF1, emu.cpu.IM, emu.cpu.PC)
}
