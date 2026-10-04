package main

import (
	"fmt"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollCPUProbe — what does the Z80 itself see at the table addrs?
// Probe code runs FOR REAL; results land in $9000 scratch. If CPU sees the
// baked table bytes, the corruption is test-tooling read-path only; if the
// CPU sees junk, the demo's sprite/table fetches are junk = the rendering
// bug Seva sees.
func TestSinescrollCPUProbe(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	// code: LD A,($9A20) / LD ($9000),A / LD HL,($9A22) / LD ($9001),HL / HALT
	probe := []byte{
		0x3A, 0x20, 0x9A, // ld a,($9A20)
		0x32, 0x00, 0x90, // ld ($9000),a
		0x2A, 0x22, 0x9A, // ld hl,($9A22)
		0x22, 0x01, 0x90, // ld ($9001),hl
		0x76, // halt
	}
	for i, v := range probe {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	// write KNOWN pattern at the table address through the same API first
	for i := 0; i < 6; i++ {
		emu.mem.Write(0x9A20+uint16(i), byte(0xC0+i))
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000
	fmt.Printf("SET pc=%04X read8000=%02X im=%d iff1=%v halted=%v\n",
		emu.cpu.PC, emu.mem.Read(0x8000), 0, emu.cpu.IFF1, emu.cpu.Halted)
	for i := 0; i < 8; i++ {
		emu.cpu.StepInstructionWithIRQ()
		fmt.Printf("step%d pc=%04X A=%02X sp=%04X\n", i, emu.cpu.PC, emu.cpu.A, emu.cpu.SP)
	}
	fmt.Printf("PROBE pc=%04X scratch: A=%02X (want C0) HL=%04X HL@9001=%02X%02X (want C1..C5->%04X)\n",
		emu.cpu.PC, emu.mem.Read(0x9000), 0xC1|(0xC2<<8), emu.mem.Read(0x9001), emu.mem.Read(0x9002), 0xC2C1)
}
