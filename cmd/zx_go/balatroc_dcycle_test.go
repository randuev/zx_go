package main

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocStatsDCyclePlus2(t *testing.T) {
	if os.Getenv("DCYCLE") == "" {
		t.Skip("set DCYCLE=1")
	}
	tap := "/root/nerve-workspace/demos/balatroc-c/build/board.tap"
	tapRaw, err := os.ReadFile(tap)
	if err != nil {
		t.Fatal(err)
	}
	mapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.map")
	syms := map[string]uint16{}
	re := regexp.MustCompile(`_(\w+)\s+=\s+\$([0-9A-Fa-f]{4})`)
	for _, m := range re.FindAllStringSubmatch(string(mapRaw), -1) {
		var v uint64
		fmt.Sscanf(m[2], "%x", &v)
		syms[m[1]] = uint16(v)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var boardData []byte
	var boardAddr uint16
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			boardData, boardAddr = blocks[i+1].data, b.start
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = boardAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF(150)
	stroke(4, 0x01, 120) // START
	gs := syms["g_stats"]
	lit := func(y0, y1, bx0, bx1 int) int {
		n := 0
		for y := y0; y <= y1; y++ {
			for bx := bx0; bx <= bx1; bx++ {
				a := uint16(0x4000 + ((y&7)<<8) + ((y&0x38)<<2) + ((y&0xC0)<<5) + bx)
				if emu.mem.Read(a) != 0 {
					n++
				}
			}
		}
		return n
	}
	stroke(7, 0x04, 60)
	stroke(7, 0x04, 60)
	for round := 1; round <= 4; round++ {
		stroke(1, 0x04, 120) // D
		aw := 0
		if emu.mem.Read(0x5800+3*32+10) == 0x07 || emu.mem.Read(0x5800+3*32+10) == 0x47 {
			aw++
		}
		fmt.Printf("DCYCLE #%d g_stats=%d sprCol=%d whiteAttr=%d PC=$%04X\n",
			round, emu.mem.Read(gs), lit(104, 167, 0, 0), aw, emu.cpu.PC)
		if round%2 == 0 { // after a CLOSE expect sprite col empty
			spr := lit(104, 167, 0, 0)
			if spr != 0 {
				t.Errorf("close #%d left %d sprite-column bytes lit", round/2, spr)
			}
		}
	}
}
