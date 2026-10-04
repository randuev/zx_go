package main

import (
	"fmt"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSinescrollBankWindowMap(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		bbk byte
		src byte
	}{
		{0x07, 0xA5}, // page7: $C000 = b7
		{0x0D, 0x5A}, // page5: $C000 = b5
	} {
		// OUT $7FFD value via OUTI trick like the ISR
		emu.mem.Write(0x9000, tc.bbk)
		// page via the real ULA port path, then write through $C000 window
		emu.ula.WritePort(0x7FFD, tc.bbk)
		emu.mem.Write(0xC000, tc.src)
		b5 := emu.mem.RAM8KPage(10)[0]
		b7 := emu.mem.RAM8KPage(14)[0]
		fmt.Printf("bbk=%02X write->$C000=%02X b5[0]=%02X b7[0]=%02X\n", tc.bbk, tc.src, b5, b7)
		pageBank := int(tc.bbk & 7)
		got := map[int]byte{5: b5, 7: b7}[pageBank]
		if got != tc.src {
			t.Fatalf("bbk=%02X: $C000 does NOT map page bank %d (got %02X)", tc.bbk, pageBank, got)
		}
	}
}
