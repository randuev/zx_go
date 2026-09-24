package main

// One-shot dual-bank census (delete when done): full-width 0..255 pixel ASCII
// around the Crux stars + canton, plus attr grids, for BOTH screen banks, so
// "page missing star", "flashing attrs" and "4th star wrong side" are all
// answered from RAM truth.

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestCruxayBothProbe(t *testing.T) {
	tapPath := "/root/nerve-workspace/demos/cruxay/cruxay_v20h.tap"
	if p := os.Getenv("CRUXAY_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	var blk []byte
	var loadAddr uint16
	off := 0
	for off+2 <= len(raw) {
		n := int(binary.LittleEndian.Uint16(raw[off:]))
		off += 2
		if off+n > len(raw) {
			break
		}
		payload := raw[off : off+n]
		off += n
		if len(payload) < 16 {
			continue
		}
		if payload[0] == 0 && payload[1] == 3 {
			loadAddr = binary.LittleEndian.Uint16(payload[14:16])
			continue
		}
		if payload[0] == 0xFF && loadAddr != 0 && blk == nil {
			blk = payload[1 : n-1]
		}
	}
	if blk == nil {
		t.Fatal("no CODE block")
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
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00)
	emu.cpu.PC = loadAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	for i := 0; i < 200; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("ScreenPage=%d", emu.mem.ScreenPage)

	// bank n physical 8K pages: b5 -> 10,11; b7 -> 14,15.
	bank := func(n int) ([2][]byte, [2][]byte) {
		px := [2][]byte{emu.mem.RAM8KPage(2*n), emu.mem.RAM8KPage(2*n + 1)}
		return px, px
	}
	pixAt := func(px [2][]byte, y, x int) byte {
		abs := (y&7)*0x100 + ((y>>3)&7)*0x20 + (y>>6)*0x800 + x/8
		return px[abs/0x2000][abs%0x2000]
	}
	b5, _ := bank(5)
	b7, _ := bank(7)
	// attrs live offset 0x1800 (pix pages) — bank attr pointer same.
	// Star-blob centers: scan rows outside the canton (x>=136), group lit
	// runs into blobs, report (cx,cy,runwidth). Rows with cross diagonals
	// excluded by ignoring narrow diagonal noise via run continuity.
	blobs := func(name string, px [2][]byte) {
		type blob struct{ x0, x1, y0, y1 int }
		var bs []blob
		for y := 8; y < 184; y++ {
			run := -1
			for x := 136; x < 256; x++ {
				lit := pixAt(px, y, x)&(1<<uint(7-x%8)) != 0
				if lit && run < 0 {
					run = x
				}
				if (!lit || x == 255) && run >= 0 {
					end := x - 1
					if x == 255 && lit {
						end = 255
					}
					if end-run <= 15 { // plus-stars max 13px wide
						placed := false
						for i := range bs {
							b := &bs[i]
							if y >= b.y0 && y <= b.y1+1 && run <= b.x1+1 && end >= b.x0-1 {
								b.x0 = min(b.x0, run)
								b.x1 = max(b.x1, end)
								b.y1 = y
								placed = true
								break
							}
						}
						if !placed {
							bs = append(bs, blob{run, end, y, y})
						}
					}
					run = -1
				}
			}
		}
		for _, b := range bs {
			if b.y1-b.y0 < 2 { // ignore single-row diagonal specks
				continue
			}
			t.Logf("%s STAR cx=%d cy=%d x=%d..%d y=%d..%d w=%d h=%d", name,
				(b.x0+b.x1)/2, (b.y0+b.y1)/2, b.x0, b.x1, b.y0, b.y1, b.x1-b.x0+1, b.y1-b.y0+1)
		}
	}
	blobs("b5", b5)
	blobs("b7", b7)

	// attr grids rows 0..7 x cols 0..31 (each col = 8px)
	attr := func(n int, r int) string {
		p := emu.mem.RAM8KPage(2*n + (0x1800+0xFF)/0x2000)
		off := (0x1800+r*32)%0x2000
		s := ""
		for c := 0; c < 32; c++ {
			v := p[off+c]
			if v == 0x0F {
				s += "."
			} else if v == 0x17 {
				s += "R"
			} else {
				s += fmt.Sprintf("%02X", v)
			}
		}
		return s
	}
	for _, n := range []int{5, 7} {
		for r := 3; r <= 8; r++ {
			t.Logf("attr b%d r%d %s", n, r, attr(n, r))
		}
	}
}
