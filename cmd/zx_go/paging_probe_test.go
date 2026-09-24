package main

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestPagingProbe answers, on the +2 core: when pg writes $05 / $0D / $0F
// to $7FFD, which physical bank do CPU writes at $4000 and $C000 land in,
// as observed through RAM8KPage(bank*2)?
func TestPagingProbe(t *testing.T) {
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
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	rd := func(p int, off int) byte {
		h := emu.mem.RAM8KPage(p * 2)
		if h == nil {
			return 0xEE
		}
		if off < len(h) {
			return h[off]
		}
		return 0xDD
	}
	for _, val := range []byte{0x05, 0x0D, 0x0F, 0x07} {
		emu.mem.PageMemory(val)
		emu.mem.Write(0xC000, 0xA0|val)
		emu.mem.Write(0xC840, 0xC0|val) // char row 10-ish via slot
		emu.mem.Write(0x4000, 0x40|val)
		emu.mem.Write(0x4840, 0xB0|val)
		t.Logf("pg=%02X ScreenPage=%d  b5[0]=%02X b5[840]=%02X b7[0]=%02X b7[840]=%02X",
			val, emu.mem.ScreenPageIndex(), rd(5, 0), rd(5, 0x840), rd(7, 0), rd(7, 0x840))
	}
}
