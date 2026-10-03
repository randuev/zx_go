package main

import (
	"fmt"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestPAWindowProbe: seed b5@chip10=$55, b7@chip14=$77 via direct chip pokes,
// then read $4000 / $C000 from the CPU under PA=0 and PA=1.
func TestPAWindowProbe(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 10; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	copy(emu.mem.RAM8KPage(10)[:6144], bytes55)
	copy(emu.mem.RAM8KPage(14)[:6144], bytes77)
	// page to PA=0 ($07) then PA=1 ($0F); CPU reads $4000 and $C000
	for _, pg := range []byte{0x07, 0x0F} {
		prog := []byte{0x3E, pg, 0x21, 0x00, 0x80, 0x01, 0xFD, 0x7F, 0xED, 0x79,
			0x3A, 0x00, 0x40, 0x32, 0x00, 0x80, // ld a,($4000) ; ld ($8000),a
			0x3A, 0x00, 0xC0, 0x32, 0x01, 0x80, // ld a,($C000) ; ld ($8001),a
			0x76}
		for i, b := range prog {
			emu.mem.Write(0x8010+uint16(i), b)
		}
		emu.cpu.PC = 0x8010
		for i := 0; i < 12; i++ {
			emu.cpu.StepInstructionWithIRQ()
		}
		fmt.Printf("pg=$%02X: CPU read $4000=%02X $C000=%02X (chips b5=%02X b7=%02X)\n",
			pg, emu.mem.Read(0x8000), emu.mem.Read(0x8001),
			emu.mem.RAM8KPage(10)[0], emu.mem.RAM8KPage(14)[0])
	}
}

var bytes55 = func() []byte { b := make([]byte, 6144); for i := range b {
	b[i] = 0x55
}; return b }()
var bytes77 = func() []byte { b := make([]byte, 6144); for i := range b {
	b[i] = 0x77
}; return b }()
