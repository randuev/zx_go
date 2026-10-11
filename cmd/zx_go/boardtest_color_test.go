package main

import (
	"bufio"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

var pal = [16]int{0x00, 0x00, 0x00, 0x00, 0xD7, 0x00, 0xD7, 0xD7, 0x00,
	0xFF, 0x00, 0xFF, 0x00, 0xFF, 0xFF, 0xFF} // black ink pairs placeholder — replaced below

// TestBoardtestColor : full mock->native 320x192 RGBA comparison render.
func TestBoardtestColor(t *testing.T) {
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
	// ZX palette RGBA8888
	palRGBA := [16][3]int{
		{0, 0, 0}, {0, 0, 0xD7}, {0xD7, 0, 0}, {0xD7, 0, 0xD7},
		{0, 0xD7, 0}, {0, 0xD7, 0xD7}, {0xD7, 0xD7, 0}, {0xD7, 0xD7, 0xD7},
		{0, 0, 0xFF}, {0, 0, 0xFF}, {0xFF, 0, 0}, {0xFF, 0x6A, 0x6A},
		{0, 0xFF, 0}, {0x6A, 0xFF, 0x6A}, {0xFF, 0xFF, 0}, {0xFF, 0xFF, 0xFF},
	}
	pb := func(y int) uint16 {
		return 0x4000 + uint16((y&7)<<8+(y&0x38)<<2+(y&0xC0)<<5)
	}
	out, err := os.Create("/tmp/cur_full.rgba")
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(out)
	for y := 0; y < 192; y++ {
		for c := 0; c < 32; c++ {
			b := balPeek(emu, pb(y)+uint16(c))
			at := balPeek(emu, 0x5800+uint16((y>>3)*32+c))
			ink := int(at & 7)
			pap := int((at >> 3) & 7)
			if at&0x80 != 0 {
				ink ^= 8
				pap ^= 8
			}
			for px := 0; px < 8; px++ {
				col := pap
				if b&(0x80>>uint(px)) != 0 {
					col = ink
				}
				p := palRGBA[col]
				w.WriteByte(byte(p[0]))
				w.WriteByte(byte(p[1]))
				w.WriteByte(byte(p[2]))
				w.WriteByte(255)
			}
		}
	}
	w.Flush()
	out.Close()
}
