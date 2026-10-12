package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestAttrFlashZero : every attribute byte of boardtest and balatro after a
// full render must have FLASH bit (0x80, ZX hardware standard: ink 2-0,
// paper 5-3, bright bit 6, FLASH bit 7) CLEAR. The old core decoded bright
// as bit 7, which hid our $B0/$B2… literals (true flash) as static bright.
// This gate fails loudly if any flash byte returns.
func TestAttrFlashZero(t *testing.T) {
	run := func(t *testing.T, path string) map[uint16]uint16 {
		bin, err := os.ReadFile(path)
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
		bad := map[uint16]uint16{}
		for a := 0x5800; a < 0x5800+768; a++ {
			v := balPeek(emu, uint16(a))
			if v&0x80 != 0 {
				bad[uint16(a)] = uint16(v)
			}
		}
		return bad
	}
	for _, tc := range []struct{ name, path string }{
		{"boardtest", "/root/nerve-workspace/demos/balatro/boardtest.bin"},
		{"balatro", "/root/nerve-workspace/demos/balatro/balatro.bin"},
	} {
		if _, err := os.Stat(tc.path); err != nil {
			t.Skip(tc.path)
		}
		bad := run(t, tc.path)
		for a, v := range bad {
			t.Errorf("%s FLASH ARMED attr $%04X=$%02X (row %d col %d)", tc.name, a, v, (a-0x5800)/32, (a-0x5800)%32)
		}
		fmt.Printf("%s : attr flash bytes=%d / 768\n", tc.name, len(bad))
	}
}
