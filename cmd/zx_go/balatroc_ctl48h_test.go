package main

// balatroc_ctl48h_test.go — SEVA report 2026-10-19: "instructions are not
// showing correct keys ... when I try to play a hand there is no error
// message like before".
// (1) LABEL LAW: key_lbl reads the packed KCH grid at stride 2; a single-
// char label that occupies ONE byte shifts every decode. The gate reads the
// shipped tap and asserts the 80-byte padded grid + the six default action
// labels decode to 5 8 7 0 6 S.
// (2) BANNER LAW: since the 48e restructure nothing stamped band 22 during
// the round poll (only draw_board at ROUND start could) — every mid-round
// error banner was born invisible. banner_at now paints immediately; the
// gate boots live (crt0), STARTs round 1, strokes PLAY with NOTHING
// selected, and demands the band-22 strip turns ink + yellow-paper attr.
// Constants: board.map of tap 3ec1d25a… (_tgt=$F9FD _hands_l=$F9F9
// _banner_q=$FB88 _bmsg=$FB89).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocCtl48hPlus2(t *testing.T) {
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	// --- (1) label grid in the SHIPPED binary ---
	// build the grid the same way the law defines it: every label exactly
	// two bytes — typed literals lose boundary spaces (they did, twice).
	kb := [][]string{
		{"CS", "Z", "X", "C", "V"},
		{"A", "S", "D", "F", "G"},
		{"Q", "W", "E", "R", "T"},
		{"1", "2", "3", "4", "5"},
		{"0", "9", "8", "7", "6"},
		{"P", "O", "I", "U", "Y"},
		{"EN", "L", "K", "J", "H"},
		{"SP", "SY", "M", "N", "B"},
	}
	var gs []byte
	for _, row := range kb {
		for _, l := range row {
			gs = append(gs, l...)
			if len(l) == 1 {
				gs = append(gs, ' ')
			}
		}
	}
	if len(gs) != 80 {
		t.Fatalf("grid construction wrong: %d bytes", len(gs))
	}
	i := -1
	for j := 0; j+80 <= len(tapRaw); j++ {
		ok := true
		for k := 0; k < 80; k++ {
			if tapRaw[j+k] != gs[k] {
				ok = false
				break
			}
		}
		if ok {
			i = j
			break
		}
	}
	if i < 0 {
		t.Fatal("padded KCH grid not found in tap — label stride still misaligned")
	}
	lbl := func(r, b byte) string {
		c := byte(0)
		for c < 5 && (b&(1<<c)) == 0 {
			c++
		}
		return string(tapRaw[i+(int(r)*5+int(c))*2 : i+(int(r)*5+int(c))*2+2])
	}
	// kmap defaults: LEFT=(3,0x10) RIGHT=(4,0x04) SEL=(4,0x08) PLAY=(4,0x01)
	// DISC=(4,0x10) SORT=(1,0x02)
	want := []struct {
		name    string
		r, b    byte
		pref    string // label must START with this char
	}{
		{"LEFT", 3, 0x10, "5"}, {"RIGHT", 4, 0x04, "8"}, {"SEL", 4, 0x08, "7"},
		{"PLAY", 4, 0x01, "0"}, {"DISC", 4, 0x10, "6"}, {"SORT", 1, 0x02, "S"},
	}
	for _, w := range want {
		got := lbl(w.r, w.b)
		if len(got) < 1 || got[0:1] != w.pref {
			t.Fatalf("label %s decodes %q — must start %q", w.name, got, w.pref)
		}
		fmt.Printf("CTL48H label %s=%q OK\n", w.name, got)
	}

	// --- (2) banner paints live on invalid PLAY ---
	const tgt = 0xF9FD
	const handsL = 0xF9F9
	const bannerQ = 0xFB88
	emu := balBootLive(t)
	runF := func(n int) {
		for j := 0; j < n; j++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	runF(160)                  // crt0 -> title poll
	balStroke(emu, runF, 4, 0x01, 200) // 0 = START -> round 1 deals
	runF(140)
	if emu.cpu.PC < 0x8000 || emu.cpu.PC > 0xF000 {
		t.Fatalf("not inside the game after START: PC=$%04X", emu.cpu.PC)
	}
	bandInk := func() int {
		n := 0
		for y := 176; y <= 183; y++ {
			for bx := 0; bx < 32; bx++ {
				if emu.mem.Read(uint16(0x4000+((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5)+bx)) != 0 {
					n++
				}
			}
		}
		return n
	}
	bandYellow := func() int {
		n := 0
		for bx := 0; bx < 32; bx++ {
			if emu.mem.Read(uint16(0x5800+22*32+bx))&0x38 == 0x30 { // paper yellow
				n++
			}
		}
		return n
	}
	ink0, yel0 := bandInk(), bandYellow()
	fmt.Printf("CTL48H round1 idle: band22 ink=%d yellow=%d\n", ink0, yel0)
	// PLAY with nothing selected -> STR_NOSEL must PAINT NOW
	stroke(4, 0x01, 30)
	ink1, yel1 := bandInk(), bandYellow()
	bq := emu.mem.Read(bannerQ)
	fmt.Printf("CTL48H after PLAY@0sel: banner_q=%d band22 ink=%d yellow=%d PC=$%04X\n",
		bq, ink1, yel1, emu.cpu.PC)
	if ink1 == 0 || yel1 == 0 {
		savePNGAtomic("/tmp/ctl48h_nosel.png", emu.lastFrame())
		t.Fatalf("STR_NOSEL banner never stamped: ink=%d yellow=%d", ink1, yel1)
	}
	savePNGAtomic("/tmp/ctl48h_nosel.png", emu.lastFrame())
	// NO HANDS LEFT banner too: zero the counter, select one card, PLAY
	emu.mem.Write(handsL, 0)
	stroke(4, 0x08, 20) // SEL the cursor card
	if nSel := emu.mem.Read(0xFC6F); nSel == 0 {
		stroke(4, 0x04, 20) // step onto a dealt card, then try again
		stroke(4, 0x08, 20)
	}
	stroke(4, 0x01, 30) // PLAY -> NO HANDS LEFT
	if emu.mem.Read(bannerQ) == 0 {
		t.Fatalf("NOHANDS banner did not arm: banner_q=0 hands_l=%d", emu.mem.Read(handsL))
	}
	fmt.Println("CTL48H PASS: labels decode 5 8 7 0 6 S from tap; banners stamp on event")
}
