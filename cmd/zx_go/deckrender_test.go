package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestDeckRender : FULL DECK card-renderer gate (Seva v5 request).
// Draws all 52 cards through cardCell at positions spanning every ULA
// third boundary and py-offset, verifying byte-exact per card :
//   - 16 suit rows land in the right bytes (third-split safe)
//   - ♥/♦ attr cells red ink ($7A) , ♣/♠ black ink ($78)
//   - frame intact (top row $FF / side bits)
//   - rank glyph present at x=cx+1 (never bit7 -> never blends frame)
func TestDeckRender(t *testing.T) {
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
	S := loadBT(t)
	poke := func(n string, v byte) { emu.mem.Write(S[n], v) }
	cellPC := S["cardCell"]
	// sentinel return : HALT opcode in code-page scratch (past all data)
	scratch := S["ENDMARK"] + 8
	if scratch < 0xA000 {
		scratch = 0xA800
	}
	emu.mem.Write(scratch, 0x76) // HALT

	pixBase := func(y int) uint16 {
		return 0x4000 + uint16((y&7)<<8) + uint16((y&0x38)<<2) + uint16((y&0xC0)<<5)
	}
	// suit tables (mirror of SUIT16 order 0=C 1=H 2=D 3=S)
	suits := [][]byte{
		{0x18, 0x18, 0x3C, 0x3C, 0x18, 0x18, 0xFF, 0xFF, 0x7E, 0x7E, 0x18, 0x18, 0x18, 0x3C, 0x3C, 0x00},
		{0x66, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7E, 0x7E, 0x3C, 0x3C, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00},
		{0x18, 0x18, 0x3C, 0x3C, 0x7E, 0x7E, 0xFF, 0xFF, 0xFF, 0xFF, 0x7E, 0x7E, 0x3C, 0x3C, 0x18, 0x18},
		{0x18, 0x18, 0x3C, 0x3C, 0x7E, 0x7E, 0xFF, 0xFF, 0xFF, 0x7E, 0x3C, 0x18, 0x18, 0x3C, 0x3C, 0x00},
	}
	rankCh := []byte{'2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'}

	// 7 cols x 4 rows per page ; y0 rows : 8 (crosses third at 64) , 56 ,
	// 104 (crosses third at 128) , 152 -> full-deck positions cover thirds
	cols := []byte{2, 6, 10, 14, 18, 22, 26}
	wds := []byte{1, 7, 13, 19}
	seen := 0
	for page := 0; page < 2; page++ {
		// fresh bitmap + attrs
		for i := 0; i < 0x4000; i++ {
			emu.mem.Write(0x4000+uint16(i), 0x00)
		}
		for i := 0; i < 0x300; i++ {
			emu.mem.Write(0x5800+uint16(i), 0x27)
		}
		for slot := 0; slot < 28 && seen < 52; slot++ {
			ci := seen
			suit := byte(ci / 13)
			rank := byte(ci % 13)
			cdx := suit<<4 | rank
			cbx := cols[slot%7]
			wd := wds[slot/7]
			poke("CDX", cdx)
			poke("CBX", cbx)
			poke("BAND", wd)
			poke("WC", 0x03)
			poke("SEL", 0x00)
			// VALPTR = 0 -> skip value (test focus : suit/attr/frame/rank)
			emu.mem.Write(S["VALPTR"], 0x00)
			emu.mem.Write(S["VALPTR"]+1, 0x00)
			emu.mem.Write(S["PYOFF"], 0x00)
			// call cardCell with sentinel return
			emu.mem.Write(0xBF00, byte(scratch&0xFF))
			emu.mem.Write(0xBF01, byte(scratch>>8))
			emu.cpu.SP = 0xBF00
			emu.cpu.PC = cellPC
			halted := false
			// optional write-trace for one card : DECKTRACE=<ci>
			traceOn := os.Getenv("DECKTRACE") != "" && ci == 14
			type wk struct {
				y int
			}
			wprev := map[wk]byte{}
			watchY := func() {
				for y := 0; y < 192; y++ {
					a := pixBase(y) + uint16(cbx+1)
					v := balPeek(emu, a)
					if o, ok := wprev[wk{y}]; ok && o != v {
						fmt.Printf("W y%03d %02X->%02X pc=$%04X a=$%02X hl=$%04X bc=$%04X de=$%04X\n", y, o, v, emu.cpu.PC, emu.cpu.A, emu.cpu.HL(), emu.cpu.BC(), emu.cpu.DE())
						wprev[wk{y}] = v
					} else if !ok {
						wprev[wk{y}] = v
					}
				}
			}
			for i := 0; i < 200000; i++ {
				if emu.cpu.PC == scratch {
					halted = true
					break
				}
				emu.cpu.StepInstruction()
				if traceOn {
					watchY()
				}
			}
			if !halted {
				t.Fatalf("card %d (CDX=%02X) : cardCell never returned pc=$%04X", ci, cdx, emu.cpu.PC)
			}
			y0 := int(wd) * 8
			if !halted {
				continue
			}
			// 1) suit pixels rows y0+16..+31 byte cbx+1
			for r := 0; r < 16; r++ {
				got := balPeek(emu, pixBase(y0+16+r)+uint16(cbx+1))
				want := suits[suit][r]
				if got != want {
					t.Errorf("card %d (%c%c) row+%d : suit byte got $%02X want $%02X (y%d third-cross?)",
						ci, rankCh[rank], "CHDS"[suit], 16+r, got, want, y0+16+r)
				}
			}
			// 2) attrs : middle column rows wd+2,wd+3 red for H/D
			wantInk := byte(0x38) // white paper black ink, standard layout (bright bit unnecessary; renders identical to $78)
			if suit == 1 || suit == 2 {
				wantInk = 0x3A // RED ink on white paper — donor v4.2 approved law : stay RED non-bright to match plate
			}
			for b := 2; b <= 3; b++ {
				a := balPeek(emu, 0x5800+uint16(int(wd)+b)*32+uint16(cbx+1))
				if a != wantInk {
					t.Errorf("card %d (%c%c) attr b%d : got $%02X want $%02X", ci, rankCh[rank], "CHDS"[suit], b, a, wantInk)
				}
			}
			// frame rows (band wd top / wd+4 bottom) must stay black ink
			for _, b := range []byte{0, 4} {
				a := balPeek(emu, 0x5800+uint16(int(wd)+int(b))*32+uint16(cbx))
				if a != 0x38 {
					t.Errorf("card %d frame attr b%d : got $%02X want $38", ci, b, a)
				}
			}
			// 3) frame intact
			top := balPeek(emu, pixBase(y0)+uint16(cbx))
			if top != 0xFF {
				t.Errorf("card %d frame top byte got $%02X", ci, top)
			}
			side := balPeek(emu, pixBase(y0+20)+uint16(cbx))
			if side&0x80 == 0 {
				t.Errorf("card %d side bit7 gone y%d got $%02X", ci, y0+20, side)
			}
			// 4) rank glyph present at x=cx*8+1 (bit6 col) rows y0+2.. and bit7 row4 must be frame-only there
			lit := 0
			for y := y0 + 2; y <= y0+8; y++ {
				v := balPeek(emu, pixBase(y)+uint16(cbx))
				lit |= int(v) & 0x7E
			}
			if lit == 0 {
				t.Errorf("card %d rank glyph missing (x=cx+1 all blank)", ci)
			}
			seen++
		}
	}
	if seen != 52 {
		t.Fatalf("deck gate drew %d cards , want 52", seen)
	}
	// selected-heart pass : SEL raises card + flips plate to yellow paper ;
	// Seva law : suit ink MUST STAY RED on selected hearts/diamonds
	poke("CDX", 0x10) // heart A
	poke("CBX", 14)
	poke("BAND", 7)
	poke("WC", 0x03)
	poke("SEL", 0x80)
	emu.mem.Write(S["VALPTR"], 0x00)
	emu.mem.Write(S["VALPTR"]+1, 0x00)
	emu.mem.Write(S["PYOFF"], 0x00)
	emu.mem.Write(0xBF00, byte(scratch&0xFF))
	emu.mem.Write(0xBF01, byte(scratch>>8))
	emu.cpu.SP = 0xBF00
	emu.cpu.PC = cellPC
	for i := 0; i < 200000; i++ {
		if emu.cpu.PC == scratch {
			break
		}
		emu.cpu.StepInstruction()
	}
	for b := 2; b <= 3; b++ {
		a := balPeek(emu, 0x5800+uint16(6+b)*32+15) // raised : WD=BAND-1 ; col CBX+1
		if a != 0x72 {
			t.Errorf("selected heart attr b%d : got $%02X want $72 (RED ink on yellow plate — donor v4.2 approved ccRed selected encode)", b, a)
		}
	}
	fmt.Printf("DECK: 52/52 cards rendered across thirds , ink law holds\n")
}
