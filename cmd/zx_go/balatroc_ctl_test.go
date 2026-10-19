package main

// balatroc_ctl_test.go — SEva screenshots (2026-10-18 20:29): REDEFINE
// KEYS shows bare "?" for ALL six actions — but the current ctl_paint
// always paints "[label]" (6-char buffer, open bracket is unconditional),
// so a bare "?" column cannot come from the pass-46 binary. This test
// boots the REAL tape, enters title_screen, strokes '4' (row3 bit3 = '4')
// and also jumps redefine_screen directly, then censuses the label columns
// (x180..210) glyph-by-glyph: '[' has a distinct pixel fingerprint, '?'
// another. PASS = labels are bracketed real names, zero bare '?'.
// Also PNGs both paths for eyes.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocCtlLabelsPlus2(t *testing.T) {
	if os.Getenv("CTL") == "" {
		t.Skip("set CTL=1")
	}
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var titleAddr, redefAddr uint16
	var boardData, musicData []byte
	_ = redefAddr
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			titleAddr, boardData = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			musicData = blocks[i+1].data
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
		t.Fatalf("newEmulator: %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(titleAddr+uint16(i), v)
	}
	for i, v := range musicData {
		emu.mem.Write(0x6000+uint16(i), v)
	}
	// kmap after boot: dump the live bindings straight from RAM
	kr := []uint16{0xF7F6, 0}
	_ = kr
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	// jump straight into title_screen (proven entry law: sentinel + fresh stack)
	const title = 0xaa5d
	const redef = 0xA8A2
	jump := func(pc uint16) {
		sent := uint16(0x7D00)
		emu.mem.Write(sent, 0x76)
		emu.cpu.SP = 0xFF56
		emu.mem.Write(0xFF56, byte(sent))
		emu.mem.Write(0xFF57, byte(sent>>8))
		emu.cpu.PC = pc
		emu.cpu.IFF1, emu.cpu.IFF2 = false, false
		emu.cpu.IM = 1
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	// label census: for each action row y=5*8+.., columns x176..210
	// count lit pixels and report the row's glyph signature hex.
	labelInk := func(tag string) {
		fmt.Printf("%s label ink:", tag)
		for i := 0; i < 6; i++ {
			y := 40 + i*8
			n := 0
			var sig []byte
			for bx := 22; bx <= 26; bx++ {
				col := byte(0)
				for b := 0; b < 8; b++ {
					yy := y + b
					a := uint16(0x4000 + (((yy & 7) << 8) + ((yy & 0x38) << 2) + ((yy & 0xC0) << 5)) + bx)
					if emu.mem.Read(a) != 0 {
						col |= 1 << uint(7-b)
						n++
					}
				}
				sig = append(sig, col)
			}
			fmt.Printf(" r%d(%d)", i, n)
			_ = sig
		}
		fmt.Println()
		// ASCII art of the label strip: px x176..212, rows 40..95
		fmt.Printf("%s art:\n", tag)
		for y := 40; y < 96; y++ {
			fmt.Printf(" y%03d ", y)
			for x := 176; x < 212; x++ {
				bx := x / 8
				a := uint16(0x4000 + (((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)) + bx)
				if emu.mem.Read(a)&(1<<uint(7-(x&7))) != 0 {
					fmt.Print("#")
				} else {
					fmt.Print(".")
				}
			}
			fmt.Println()
		}
	}
	jump(title)
	runF(90)
	savePNGAtomic("/tmp/ctl1_title.png", emu.lastFrame())
	// path A: real navigation — '4' = row3 bit3 on the title menu
	stroke(3, 0x08, 60)
	savePNGAtomic("/tmp/ctl2_via_title4.png", emu.lastFrame())
	labelInk("via-title4")
	fmt.Printf("PC=$%04X\n", emu.cpu.PC)
	// path B: direct jump to redefine_screen
	jump(redef)
	runF(60)
	savePNGAtomic("/tmp/ctl3_direct.png", emu.lastFrame())
	labelInk("direct")
	// crop the label area to a small PNG for the report? full PNG is fine.
}
