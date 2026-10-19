package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocProbePlus2(t *testing.T) {
	if os.Getenv("PROBE") == "" { t.Skip("set PROBE=1") }
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil { t.Fatal(err) }
	emu.paused.Store(true)
	for i := 0; i < 220; i++ { runOneFrameHeadless(emu, roms.ModelPlus2) }
	for i, v := range bd { emu.mem.Write(a+uint16(i), v) }
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFFFE
	emu.mem.Write(0xFFFC, byte(sent))
	emu.mem.Write(0xFFFD, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.Halted = false
	emu.cpu.D, emu.cpu.E, emu.cpu.H, emu.cpu.L = 0, 0, 0x80, 0x00
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) { for i := 0; i < n; i++ { runOneFrameHeadless(emu, roms.ModelPlus2) } }
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true); runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false); runF(tail)
	}
	runF(120)
	stroke(4, 0x01, 60)
	emu.mem.Write(0xFF0C, 1)
	emu.mem.Write(0xfca4, 2)
	emu.mem.Write(0xFF27, 4)
	emu.cpu.SP = 0xFFFE
	emu.mem.Write(0xFFFE, byte(sent))
	emu.mem.Write(0xFFFF, byte(sent>>8))
	emu.cpu.PC = 0xa496
	emu.cpu.Halted = false
	runF(20)
	{ var buf bytes.Buffer; _ = writeScreenshotPNG(emu,&buf); _ = os.WriteFile("/tmp/bal_probe.png",buf.Bytes(),0o644) }
	fmt.Printf("PROBE pc=$%04X sp=$%04X halted=%v shop_cur=%d jok=%d,%d\n",
		emu.cpu.PC, emu.cpu.SP, emu.cpu.Halted, emu.mem.Read(0xFF27), emu.mem.Read(0xFF0C), emu.mem.Read(0xfca4))
	lit := 0
	row := map[int]int{}
	for y := 0; y < 192; y++ {
		for b := 0; b < 32; b++ {
			v := emu.mem.Read(uint16(0x4000 + ((y&7)<<8) + ((y&0x38)<<2) + ((y&0xC0)<<5) + b))
			if v != 0 { lit++; row[y]++ }
		}
	}
	fmt.Printf("PROBE whole-screen lit=%d\n", lit)
	for _, y := range []int{136, 140, 144, 145, 160, 168, 175, 176, 180, 184} {
		fmt.Printf("PROBE row y=%d lit=%d\n", y, row[y])
	}
}
