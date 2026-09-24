package main

// One-shot static-art probe (delete when done): prove bank5 AND bank7 both
// carry the canton saltire, the white fimbriation rows, and the Commonwealth
// star body — count lit pixels in three windows per bank.

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestCruxayStaticProbe(t *testing.T) {
	tapPath := "/root/nerve-workspace/demos/cruxay/cruxay_v20i.tap"
	if p := os.Getenv("CRUXAY_TAP"); p != "" {
		tapPath = p
	}
	raw, _ := os.ReadFile(tapPath)
	var blk []byte
	var loadAddr uint16
	off := 0
	for off+2 <= len(raw) {
		n := int(binary.LittleEndian.Uint16(raw[off:]))
		off += 2
		if off+n > len(raw) {
			break
		}
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0 && p[1] == 3 {
			loadAddr = binary.LittleEndian.Uint16(p[14:16])
			continue
		}
		if p[0] == 0xFF && loadAddr != 0 && blk == nil {
			blk = p[1 : n-1]
		}
	}
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
	for i, b := range blk {
		emu.mem.Write(uint16(loadAddr)+uint16(i), b)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = loadAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	for i := 0; i < 200; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	pixAt := func(px []byte, y, x int) bool {
		abs := (y&7)*0x100 + ((y>>3)&7)*0x20 + (y>>6)*0x800 + x/8
		return px[abs]&(1<<uint(7-x%8)) != 0
	}
	count := func(px []byte, y0, y1, x0, x1 int) int {
		c := 0
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				if pixAt(px, y, x) {
					c++
				}
			}
		}
		return c
	}
	for _, pair := range []struct {
		name string
		pg   int
	}{{"b5", 10}, {"b7", 14}} {
		p := emu.mem.RAM8KPage(pair.pg)
		if p == nil {
			t.Fatalf("nil page %d", pair.pg)
		}
		t.Logf("%s saltire-diag(y20..75,x8..110)=%d fimb-rows(y40/54,x0..127)=%d star(y112..175,x32..95)=%d vfin(x56..57,y8..95)=%d",
			pair.name,
			count(p, 20, 75, 8, 110),
			count(p, 40, 40, 0, 127)+count(p, 54, 54, 0, 127),
			count(p, 112, 175, 32, 95),
			count(p, 8, 95, 56, 57))
	}
}
