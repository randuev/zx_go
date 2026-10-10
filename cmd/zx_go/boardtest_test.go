package main

import (
	"fmt"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardTest renders the STANDALONE board renderer (demos/balatro/boardtest
// v4, ROM 8x8 font) on the +2 core, proving the board appears fully formed,
// readable and stack-stable BEFORE the engine inherits the renderer.
// Ground truth = proofs/boardtest.png eyeballed vs the approved mock.
func loadBT(t *testing.T) map[string]uint16 {
	m := map[string]uint16{}
	b, err := os.ReadFile("/root/nerve-workspace/demos/balatro/boardtest.sym")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[1] == "EQU" {
			f[0] = strings.TrimSuffix(f[0], ":")
			v, err := strconv.ParseUint(strings.TrimPrefix(f[2], "0x"), 16, 32)
			if err == nil {
				m[f[0]] = uint16(v)
			}
		}
	}
	return m
}

func TestBoardTest(t *testing.T) {
	bin, err := os.ReadFile("/root/nerve-workspace/demos/balatro/boardtest.bin")
	if err != nil {
		t.Fatal(err)
	}
	S := loadBT(t)
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
	pcOps := map[uint16]*[2]int{} // pc -> [pushes,pops]
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bin {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	emu.cpu.SP = 0xBF00
	emu.cpu.PC = 0x8000
	// code+data page snapshot : any self-modification is a bug
	codeSnap := make([]byte, 0x800)
	for a := range codeSnap {
		codeSnap[a] = balPeek(emu, uint16(0x8000+a))
	}
		var bl8Log [][4]uint16
	var spr80Log [][3]uint16
	var d6Log [][3]uint16
	var shadow []uint16
	var rpixLog [][4]uint16
	var bxLog [][3]uint16
	raLog := make([][3]uint16, 0, 16)
	cardHits := 0
	calls := make([][2]uint16, 0, 64)
	var hist [16]struct {
		pc uint16
		op byte
		sp uint16
	}
	hp := 0
	halted := false
	var haltPC uint16
	deep := make([][3]uint16,0,80)
	minSP := uint16(0xFFFF)
	minSPpc := uint16(0)
	inRB := false
spLog := make([][4]uint16,0,48)
	spPrev := emu.cpu.SP
	oddSeen := false
for i := 0; i < 3000000; i++ {
		pc := emu.cpu.PC
		if pc < 0x8000 || pc >= 0x8000+uint16(len(bin)) {
			for j, c := range calls {
				t.Logf("call[%02d] $%04X -> $%04X", j, c[0], c[1])
			}
			t.Fatalf("left code range pc=$%04X i=%d SP=$%04X depth=%d", pc, i, emu.cpu.SP, len(shadow))
		}
		if pc >= S["ROWTAB"] { // entered TEXT block running as code
			for j := range hist {
				k := (hp + j) % 16
				t.Logf("slide-hist[%02d] pc=$%04X op=%02X SP=$%04X", j, hist[k].pc, hist[k].op, hist[k].sp)
			}
			t.Fatalf("pc slid into text region pc=$%04X i=%d", pc, i)
		}
		hist[hp] = struct {
			pc uint16
			op byte
			sp uint16
		}{pc, balPeek(emu, pc), emu.cpu.SP}
		hp = (hp + 1) % 16
			if balPeek(emu, pc) == 0xCD {
				tgt := uint16(balPeek(emu, pc+1)) | uint16(balPeek(emu, pc+2))<<8
				if tgt == S["blit8"] {
					sy := uint16(balPeek(emu, S["SPR"])) | uint16(balPeek(emu, S["SPR"]+1))<<8
					by := balPeek(emu, S["BX"])
					yy := balPeek(emu, S["Y"])
					if len(bl8Log) < 60 {
						bl8Log = append(bl8Log, [4]uint16{pc, uint16(sy), uint16(yy), uint16(by)})
					}
				}
				if tgt == S["drawstr6"] {
					if len(d6Log) < 40 {
						d6Log = append(d6Log, [3]uint16{pc, uint16(emu.cpu.B), uint16(emu.cpu.C)})
					}
				}
				if tgt == S["cardCell"] {
					cardHits++
					if cardHits <= 24 || cardHits%500 == 0 {
						t.Logf("cardCell call#%d pc=$%04X CBX=%02X l=%02X CDX=%02X depth=%d", cardHits, pc, balPeek(emu, S["CBX"]), uint16(emu.cpu.L), balPeek(emu, S["CDX"]), len(shadow))
					}
				}
			}
		if balPeek(emu, pc) == 0x76 { // HALT : render complete
			halted = true
			haltPC = pc
			break
		}
		if balPeek(emu, pc) == 0xCD {
			tg := uint16(balPeek(emu, pc+1)) | uint16(balPeek(emu, pc+2))<<8
			if tg == S["rectPix"] { // rectPix entry
				rpixLog = append(rpixLog, [4]uint16{pc, uint16(balPeek(emu, S["Y0"])), uint16(balPeek(emu, S["BX"])), uint16(balPeek(emu, S["W"]))})
			}
			if tg == S["pxHL"] {
				yv := balPeek(emu, S["Y"])
				if yv > 0xBF {
					for k, e := range rpixLog {
						t.Logf("rectPix[%03d] from $%04X Y0=%02X BX=%02X W=%02X", k, e[0], e[1], e[2], e[3])
					}
					t.Fatalf("pxHL bad row Y=%02X from pc=$%04X BX=%02X", yv, pc, balPeek(emu, S["BX"]))
				}
			}
		}
		if i%512 == 0 {
			// code-page integrity : report the write that self-modifies, then die
			for a := uint16(0); a < S["DSB"]-0x8000; a++ {
				got := balPeek(emu, 0x8000+a)
				if got != codeSnap[a] {
					gn := func(o uint16) int { return int(balPeek(emu, S["Y"]+o)) }
					t.Logf("SELF-MOD $%04X %02X->%02X pc=$%04X SP=$%04X depth=%d", 0x8000+a, codeSnap[a], got, pc, emu.cpu.SP, len(shadow))
					t.Logf("globals: Y=%02X Y0=%02X YY=%02X BX=%02X W=%02X VAL=%02X CDX=%02X BAND=%02X CBX=%02X RR=%02X CC=%02X",
						gn(0), gn(1), gn(2), gn(3), gn(4), gn(5), gn(9), gn(10), gn(11), gn(13), gn(14))
					t.Logf("HL=%04X DE=%04X BC=%04X A=%02X", emu.cpu.HL(), emu.cpu.DE(), emu.cpu.BC(), emu.cpu.A)
					t.Logf("shadow return addrs: %v", shadow)
					for j, c := range calls {
						t.Logf("call[%02d] $%04X -> $%04X", j, c[0], c[1])
					}
					t.Fatalf("self-modification at $%04X", 0x8000+a)
				}
			}
		}
		stkSnap := make([]byte, 0x18)
if emu.cpu.SP != spPrev {
				spLog = append(spLog, [4]uint16{pc, uint16(balPeek(emu, pc)), spPrev, emu.cpu.SP})
				if len(spLog) > 48 {
					spLog = spLog[1:]
				}
			if (emu.cpu.SP&1)==1 && !oddSeen {
				oddSeen=true
				t.Fatalf("SP ODD after pc=$%04X op=%02X (was $%04X)", pc, balPeek(emu, pc), spPrev)
			}
			spPrev = emu.cpu.SP
		}
		for k := range stkSnap {
			stkSnap[k] = balPeek(emu, uint16(0xBEE8+k))
		}
		if pc == S["raRow"] {
			raLog = append(raLog, [3]uint16{emu.cpu.HL(), uint16(balPeek(emu, S["NR"])), uint16(balPeek(emu, S["RR"]))})
			if len(raLog) > 12 {
				raLog = raLog[1:]
			}
			if emu.cpu.HL() >= 0x5B00 {
				for k, e := range raLog {
				t.Logf("raLog[%02d] hl=$%04X NR=%d RR=%d", k, e[0], e[1], e[2])
				}
				t.Fatalf("raRow hl past page")
			}
		}
		if pc == S["rectAtt"] {
			t.Logf("rectAtt-in RR=%d CC=%d NR=%d NC=%d ATT=%02X hl=$%04X", balPeek(emu, S["RR"]), balPeek(emu, S["CC"]), balPeek(emu, S["NR"]), balPeek(emu, S["NC"]), balPeek(emu, S["ATT"]), emu.cpu.HL())
			if balPeek(emu, S["NR"]) == 0 || balPeek(emu, S["NC"]) == 0 {
				t.Fatalf("rectAtt zero NR/NC")
			}
		}
		op := balPeek(emu, pc)
		if op == 0xCD {
			tgt := uint16(balPeek(emu, pc+1)) | uint16(balPeek(emu, pc+2))<<8
			if tgt == S["rectAtt"] {
				rr := balPeek(emu, S["RR"])
				nn := balPeek(emu, S["NR"])
				cc := balPeek(emu, S["CC"])
				nc := balPeek(emu, S["NC"])
				if uint16(rr)+uint16(nn) > 23 || uint16(cc)+uint16(nc) > 32 {
					t.Logf("BAD rectAtt entry RR=%d NR=%d CC=%d NC=%d i=%d", rr, nn, cc, nc, i)
				}
			}
			if tgt == S["fillRun"] {
				h := emu.cpu.HL()
				if !(h >= 0x4000 && h < 0x5B00) {
					t.Logf("first-bad fillRun @i=%d HL=$%04X A=%02X BC=%04X pc=$%04X Y=%02X BX=%02X RR=%02X CC=%02X", i, h, emu.cpu.A, emu.cpu.BC(), pc, balPeek(emu, S["Y"]), balPeek(emu, S["BX"]), balPeek(emu, S["RR"]), balPeek(emu, S["CC"]))
					for j := len(calls) - 12; j < len(calls); j++ {
						if j >= 0 {
							t.Logf("callhist[%02d] $%04X -> $%04X", j, calls[j][0], calls[j][1])
						}
					}
					t.Fatalf("bad fillRun dest")
				}
			}
			shadow = append(shadow, pc+3)
			calls = append(calls, [2]uint16{pc, tgt})
			if len(calls) > 64 {
				calls = calls[len(calls)-64:]
			}
		}
		if op == 0xC9 {
			if len(shadow) == 0 {
				t.Fatalf("RET with empty shadow stack SP=$%04X pc=$%04X", emu.cpu.SP, pc)
			}
			want := shadow[len(shadow)-1]
			got := uint16(balPeek(emu, emu.cpu.SP)) | uint16(balPeek(emu, emu.cpu.SP+1))<<8
			if want != got {
				for a := emu.cpu.SP; a < 0xBF08; a++ {
					if a < emu.cpu.SP+24 {
						t.Logf("stk[$%04X]=%02X", a, balPeek(emu, a))
					}
				}
				for j := range calls {
					t.Logf("callhist[%02d] $%04X -> $%04X", j, calls[j][0], calls[j][1])
				}
for k := len(spLog) - 1; k >= 0; k-- {
					t.Logf("splog[%02d] pc=$%04X op=%02X SP $%04X->$%04X", k, spLog[k][0], spLog[k][1], spLog[k][2], spLog[k][3])
				}
fmt.Printf("minSP=$%04X at pc=$%04X\n", minSP, minSPpc)

for k, e := range deep {
					t.Logf("deep[%02d] pc=$%04X op=%02X SP=$%04X", k, e[0], e[1], e[2])
				}
fmt.Printf("FATAL pc=$%04X SP=$%04X A=%02X BC=%04X DE=%04X HL=%04X IX=%04X IY=%04X\n", pc, emu.cpu.SP, emu.cpu.A, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL(), emu.cpu.IX, emu.cpu.IY)
				fmt.Printf("STKSP: ")
				for a := uint16(0xBEF4); a < 0xBF04; a++ {
					fmt.Printf("[%04X]=%02X ", a, balPeek(emu, a))
				}
				fmt.Printf("\n")
fmt.Printf("RAWSTK: ")
				for a := uint16(0xBEE8); a < 0xBF08; a++ {
					fmt.Printf("%02X ", balPeek(emu, a))
				}
				fmt.Printf("\n")
				fmt.Printf("wanted return $%04X ; callhist len=%d\n", want, len(calls))
							rngs := []struct{ lo, hi uint16; name string }{
				{0x802C, 0x8058, "pxHL"}, {0x8058, 0x805E, "fillRun"},
				{0x805E, 0x809A, "rectAtt"}, {0x809A, 0x80EA, "rectPix"},
				{0x80EA, 0x8173, "putc6"}, {0x8173, 0x81A9, "drawstr6"},
				{0x81A9, 0x81C6, "blit8"}, {0x81C6, 0x8271, "boxFrame"},
				{0x8271, 0x83E4, "cardCell"}, {0x83E4, 0x87BA, "renderBoard"},
			}
			for _, r := range rngs {
				var pp, po int
				for k, v := range pcOps {
					if k >= r.lo && k < r.hi { pp += v[0]; po += v[1] }
				}
				if pp != po { fmt.Printf("RGENSUS %s pushes=%d pops=%d NET=%d\n", r.name, pp, po, pp-po) }
			}
			t.Fatalf("BAD RET at $%04X SP=$%04X: stack=$%04X want $%04X depth=%d", pc, emu.cpu.SP, got, want, len(shadow))
			}
			shadow = shadow[:len(shadow)-1]
		}
		bxBefore := balPeek(emu, S["BX"])
		yBefore := balPeek(emu, S["Y"])
		y0Before := balPeek(emu, S["Y0"])
		for k := range stkSnap {
			if gotB := balPeek(emu, uint16(0xBEE8+k)); gotB != stkSnap[k] {
				t.Logf("STACK-WRITE $%04X %02X->%02X pc=$%04X SP=$%04X", 0xBEE8+uint16(k), stkSnap[k], gotB, pc, emu.cpu.SP)
			}
		}
				if op==0xF5||op==0xC5||op==0xD5||op==0xE5 || op==0xF1||op==0xC1||op==0xD1||op==0xE1 {
			c := pcOps[pc]; if c == nil { c = &[2]int{}; pcOps[pc] = c }
			if op&0x0F == 0x01 { c[1]++ } else { c[0]++ }
		}
		sprBefore := balPeek(emu, 0x8780)
			emu.cpu.StepInstruction()
			if balPeek(emu, 0x8780) != sprBefore && balPeek(emu, pc) != 0x32 && balPeek(emu, pc) != 0x77 {
				if len(spr80Log) < 24 {
					spr80Log = append(spr80Log, [3]uint16{pc, uint16(sprBefore), uint16(balPeek(emu, 0x8780))})
				}
			}
		if pc == S["renderBoard"] {
			inRB = true
		}
		if pc == S["pxHL"] && emu.cpu.SP < 0xBEE4 {
			ret0 := uint16(balPeek(emu, emu.cpu.SP)) | uint16(balPeek(emu, emu.cpu.SP+1))<<8
			ret1 := uint16(balPeek(emu, emu.cpu.SP+2)) | uint16(balPeek(emu, emu.cpu.SP+3))<<8
			ret2 := uint16(balPeek(emu, emu.cpu.SP+4)) | uint16(balPeek(emu, emu.cpu.SP+5))<<8
			t.Logf("PXHL-entry SP=$%04X ret=$%04X/%04X/%04X depth=%d", emu.cpu.SP, ret0, ret1, ret2, len(shadow))
		}
if inRB && emu.cpu.SP < 0xBEE4 {
			deep = append(deep, [3]uint16{pc, uint16(balPeek(emu, pc)), emu.cpu.SP})
			if len(deep) > 80 {
				deep = deep[1:]
			}
		}
if inRB && emu.cpu.SP < minSP {
			minSP = emu.cpu.SP
			minSPpc = pc
		}

		yAfter := balPeek(emu, S["Y"])
		y0After := balPeek(emu, S["Y0"])
		bxAfter := balPeek(emu, S["BX"])
		if bxBefore != bxAfter {
			bxLog = append(bxLog, [3]uint16{pc, uint16(bxBefore), uint16(bxAfter)})
			if len(bxLog) > 48 {
				bxLog = bxLog[1:]
			}
		}
		if (yBefore != yAfter && yAfter > 0xBF) || (y0Before != y0After && y0After > 0xBF) {
			t.Logf("Y-write $%02X->$%02X pc=$%04X SP=$%04X BX=%02X VAL=%02X A=%02X HL=%04X", yBefore, yAfter, pc, emu.cpu.SP, balPeek(emu, S["BX"]), balPeek(emu, S["VAL"]), emu.cpu.A, emu.cpu.HL())
			for j, c := range calls {
				t.Logf("call[%02d] $%04X -> $%04X", j, c[0], c[1])
			}
			t.Fatalf("bad Y write")
		}
	}
	if !halted {
		n := len(rpixLog)
		if n > 8 {
			rpixLog = rpixLog[n-8:]
		}
		for k, e := range rpixLog {
			t.Logf("last rectPix[%d] from $%04X Y0=%02X BX=%02X W=%02X", k, e[0], e[1], e[2], e[3])
		}
		for k, e := range bxLog {
			t.Logf("BXw[%02d] pc=$%04X %02X->%02X", k, e[0], e[1], e[2])
		}
		t.Fatalf("never halted; PC=$%04X depth=%d Y=%02X YY=%02X BX=%02X VAL=%02X", emu.cpu.PC, len(shadow), balPeek(emu, S["Y"]), balPeek(emu, S["YY"]), balPeek(emu, S["BX"]), balPeek(emu, S["VAL"]))
	}
	if len(shadow) != 0 {
		t.Fatalf("stack imbalance at halt: depth=%d", len(shadow))
	}
	fmt.Printf("boardtest halted pc=$%04X (clean stack)\n", haltPC)
	for _, e := range spr80Log {
		fmt.Printf("SPRWR pc=$%04X %02X->$%02X op=%02X\n", e[0], e[1], e[2], balPeek(emu, e[0]))
	}
	for _, e := range bl8Log {
		fmt.Printf("BL8 pc=$%04X SPR=$%04X Y=%d BX=%d\n", e[0], e[1], e[2], e[3])
	}
	for _, e := range d6Log {
		fmt.Printf("D6 pc=$%04X B=%d C=%d\n", e[0], e[1], e[2])
	}
	f, err := os.Create("/root/nerve-workspace/demos/balatro/proofs/boardtest.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, emu.renderFrame()); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	// ROM font probe at halt : is charset visible at $3D00 ?
	for _, ch := range []byte{'S','C','O','R','E','A'} {
		g := 0x3D00 + int(ch)*8
		row := ""
		for k := 0; k < 8; k++ {
			row += fmt.Sprintf("%02X ", balPeek(emu, uint16(g+k)))
		}
		fmt.Printf("glyph %c @%04X: %s\n", ch, g, row)
	}
	// attr census per band : every col value across the board
	for b := 10; b < 24; b++ {
		line := ""
		for c := 0; c < 32; c++ {
			line += fmt.Sprintf("%02X ", balPeek(emu, uint16(0x5800+b*32+c)))
		}
		fmt.Printf("att%02d: %s\n", b, line)
	}
	// ink census per band : proves every label/card drew pixels
	for b := 0; b < 24; b++ {
		lit := 0
		for y := b * 8; y < b*8+8; y++ {
			for x := 32; x < 256; x++ {
				a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3))
				if balPeek(emu, a) != 0 {
					lit++
				}
			}
		}
		fmt.Printf("band%02d lit=%d\n", b, lit)
	}
}
