package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatroProofX(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	for i := 0; i < 80; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	save := func(name string) {
		f, _ := os.Create("/tmp/" + name + ".png")
		png.Encode(f, emu.renderFrame())
		f.Close()
	}
	save("bal_title")
	pressOnce(t, emu, syms, "ENTER")
	for i := 0; i < 120; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	save("bal_board")
	pressOnce(t, emu, syms, "6")
	pressOnce(t, emu, syms, "6")
	for i := 0; i < 30; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	save("bal_cursor")
	t.Logf("MODE=%d NSEL=%d CURSOR=%d", balPeek(emu, syms["MODE"]), balPeek(emu, syms["NSEL"]), balPeek(emu, syms["CURSOR"]))
}
