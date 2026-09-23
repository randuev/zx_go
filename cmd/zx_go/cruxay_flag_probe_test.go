package main

// One-shot art probe (delete when done): render-truth from physical RAM on
// the +2 core. (1) attribute rows for the canton red arms — expect RED cells
// on flag rows 5-6, NOT rows 13-14 (the v20g stale-H bug); (2) ASCII bands
// of bank5 pixels: canton cross rows and the Commonwealth star silhouette.

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestCruxayFlagProbe(t *testing.T) {
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
	for i := 0; i < 150; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("settled: PC=%04X ScreenPage=%d", emu.cpu.PC, emu.mem.ScreenPage)

	// bank5: pages 10 (disp $4000-$5FFF incl attrs at +$1800) and 11.
	p10 := emu.mem.RAM8KPage(10)
	p11 := emu.mem.RAM8KPage(11)
	if p10 == nil || p11 == nil {
		t.Fatal("nil bank5 pages")
	}
	// Attrs live in p10 offset $1800 (disp $5800). Rows 5,6,13,14, cols 0..15.
	rowAttrs := func(r int) string {
		s := ""
		base := 0x1800 + r*32
		for c := 0; c < 16; c++ {
			s += fmt.Sprintf("%02X ", p10[base+c])
		}
		return s
	}
	for _, r := range []int{4, 5, 6, 7, 12, 13, 14, 15} {
		t.Logf("attr row %2d: %s", r, rowAttrs(r))
	}
	// Field attrs outside canton (col 20 rows 0..7) for comparison.
	t.Logf("attr col20 rows0-7: %02X %02X %02X %02X %02X %02X %02X %02X",
		p10[0x1800+20], p10[0x1800+32+20], p10[0x1800+64+20], p10[0x1800+96+20],
		p10[0x1800+128+20], p10[0x1800+160+20], p10[0x1800+192+20], p10[0x1800+224+20])

	// Pixel ASCII of bank5 over the whole flag: y 0..191 both page halves.
	pix := func(pages [2][]byte, y, x int) byte {
		abs := (y&7)*0x100 + ((y>>3)&7)*0x20 + (y>>6)*0x800 + x/8
		return pages[0][abs] // whole 6144px screen + attrs live in page-0 half
	}
	b5 := [2][]byte{p10, p11}
	for _, band := range [][2]int{{36, 60}, {100, 176}} {
		for y := band[0]; y <= band[1]; y++ {
			var sb []byte
			for x := 0; x < 128; x++ {
				if pix(b5, y, x)&(1<<uint(7-x%8)) != 0 {
					sb = append(sb, '#')
				} else {
					sb = append(sb, '.')
				}
			}
			t.Logf("b5 y=%3d %s", y, string(sb))
		}
	}
}
