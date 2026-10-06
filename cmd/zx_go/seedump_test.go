package main

import (
	"fmt"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSinescrollSeenDump(t *testing.T) {
	syms := loadSinescrollSyms(t)
	emu := bootSinescroll(t, syms, "/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	for i := 0; i < 400; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for b := 0; b < 8; b++ {
		bank := emu.mem.GetPage(b)
		if bank == nil {
			continue
		}
		lit := 0
		for _, x := range bank {
			if x != 0 {
				lit++
			}
		}
		fmt.Printf("bank%d lit=%d/%d\n", b, lit, len(bank))
	}
	fmt.Printf("bbk=%02X ScreenPage=%d lvl=%d p=%d\n", emu.mem.Read(syms["bbk"]),
		emu.mem.ScreenPage, emu.mem.Read(syms["lvl"]),
		int(emu.mem.Read(syms["p"]))|int(emu.mem.Read(syms["p"]+1))<<8)
}
