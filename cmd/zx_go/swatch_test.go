package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// Empirically lock attr->RGB: stamp rows with attr=(r<<3)|ink0 and read
// back the rendered pixel colors; also 2nd half tests ink values.
func TestSwatch(t *testing.T) {
	bin, err := os.ReadFile("/root/nerve-workspace/demos/balatro/swatch.bin")
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
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000
	for i := 0; i < 200000; i++ {
		emu.cpu.StepInstruction()
		if emu.cpu.PC == 0x8023 {
			break
		}
	}
	for i := 0; i < 4; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	fmt.Printf("RAM5800=%02X 5820=%02X 5838=%02X 4000=%02X pc=%04X\n",
		balPeek(emu, 0x5800), balPeek(emu, 0x5820), balPeek(emu, 0x5838), balPeek(emu, 0x4000), emu.cpu.PC)
	img := emu.renderFrame()
	for r := 0; r < 8; r++ {
		y := r*8 + 3
		c := img.At(64, y)
		rr, gg, bb, _ := c.RGBA()
		fmt.Printf("PAPER r%d attr=%02X -> RGB(%d,%d,%d)\n", r, r<<3, rr>>8, gg>>8, bb>>8)
	}
	f, _ := os.Create("/root/nerve-workspace/demos/balatro/proofs/swatch.png")
	png.Encode(f, image.NewRGBA(img.Bounds().Add(image.Pt(0,0)).Add(image.Pt(0,0))))
	f.Close()
	f2, _ := os.Create("/root/nerve-workspace/demos/balatro/proofs/swatch.png")
	png.Encode(f2, img)
	f2.Close()
}
