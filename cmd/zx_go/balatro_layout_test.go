package main

import (
	"hash/fnv"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestBalatroLayoutProof — layout-aware proof gate (coordinator spec).
// Runs until the PLAY board is painted, polls renderFrame twice 4 frames
// apart requiring equality (paint settled), then asserts:
//
//	(a) green attr census > 80% of field cells
//	(b) sidebar cmd-strip pixel row complete for its intended span
//	(c) an 8x8-ish rank glyph present at each hand-card slot
//	(d) red-ink sprite pixels > 0 with hearts/diamonds in hand
//
// Saves proof PNGs after settling.
func savePNG(t *testing.T, emu *emulator, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, emu.renderFrame()); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
}

func TestBalatroLayoutProof(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	dir := "/root/nerve-workspace/demos/balatro/proofs"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	balRunFrames(emu, 16)
	pressOnce(t, emu, syms, "SPACE")
	for i := 0; i < 200 && balPeek(emu, syms["MODE"]) != bMODE_PLAY; i++ {
		balRunFrames(emu, 1)
	}
	if m := balPeek(emu, syms["MODE"]); m != bMODE_PLAY {
		t.Fatalf("MODE=%d want PLAY", m)
	}
	hw := func() uint64 {
		f := emu.renderFrame()
		h := fnv.New64a()
		b := f.Bounds()
		raw := make([]byte, 0, b.Dx()*b.Dy()*4)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bb, aa := f.At(x, y).RGBA()
				raw = append(raw, byte(r>>8), byte(g>>8), byte(bb>>8), byte(aa>>8))
			}
		}
		h.Write(raw)
		return h.Sum64()
	}
	// settle: two captures 4 frames apart must be identical
	for i := 0; i < 200; i += 4 {
		h1 := hw()
		balRunFrames(emu, 4)
		h2 := hw()
		if h1 == h2 {
			break
		}
		if i == 196 {
			t.Fatal("paint never settled")
		}
	}

	// ---- (a) green attr census over field cells (cols 11..31, rows 0..16)
	attBase := uint16(0x5800)
	cells, green := 0, 0
	for row := 0; row <= 16; row++ {
		for col := 11; col < 32; col++ {
			cells++
			if a := balPeek(emu, attBase+uint16(row*32+col)); a&0x38 >= 0x20 && a&0x38 <= 0x38 && a&0xC0 == 0 {
				green++
			}
		}
	}
	pct := float64(green) / float64(cells) * 100
	if pct <= 80 {
		t.Fatalf("green field census %.1f%% <= 80%% (%d/%d)", pct, green, cells)
	}
	t.Logf("green census %.1f%%", pct)

	// ---- (b) cmd strip row band23: lit glyph cells across the full text span
	lit := balTextLit(emu, 23)
	if lit < 24 {
		t.Fatalf("cmd strip lit cells=%d want>=24", lit)
	}
	t.Logf("cmd strip lit cells=%d", lit)

	// ---- (c) rank glyph presence at each of the 5 hand slots
	// hand cards live near bottom bands 17-21 per drawHand; find rank glyph
	// ink by scanning each slot column window for a 5x7 ink cluster.
	// slots at cells 11+2*s (8 slots); rank glyph at y=band17*8+PYOFF2,
	// x=CBX*8+1 .. +6, rows y138..144 (rank top-left of cardCell).
	for s := 0; s < 8; s++ {
		if balPeek(emu, syms["HAND"]+uint16(s)) == 0 {
			continue
		}
		cbx := 11 + 2*s
		x0, x1 := cbx*8+1, cbx*8+7
		rowsLit := 0
		for y := 138; y < 145; y++ {
			for x := x0; x < x1; x++ {
				if balPeek(emu, ula6(y, byte(x/8)))>>(7-uint8(x%8))&1 == 1 {
					rowsLit++
					break
				}
			}
		}
		if rowsLit < 5 {
			t.Fatalf("slot %d (cell %d): rank glyph rows lit=%d want>=5", s, cbx, rowsLit)
		}
	}
	t.Logf("rank glyphs present in all %d hand slots", 8)

	// ---- (d) red suit ink > 0 with hearts/diamonds forced
	// poke hand with hearts+diamonds and redraw via game's own redraw path:
	// pressing ENTER-select then reading is stateful; simpler: poke HAND and
	// call drawHand with halt-pad return (canonical isolated-call trick).
	emu.mem.Write(syms["HAND"], 0x12) // H-rank2
	emu.mem.Write(syms["HAND"]+1, 0x23)
	emu.mem.Write(syms["HAND"]+2, 0x14)
	emu.mem.Write(syms["HAND"]+3, 0x25)
	emu.mem.Write(syms["HAND"]+4, 0x17)
	savedPC := emu.cpu.PC
	savedSP := emu.cpu.SP
	callIsolated(t, emu, syms, "drawHand")
	emu.cpu.SP = savedSP
	emu.cpu.PC = savedPC
	// count red-ink attr cells in hand band rows 17..21 cols 12..31
	redCells := 0
	for row := 17; row <= 21; row++ {
		for col := 12; col < 32; col++ {
			a := balPeek(emu, attBase+uint16(row*32+col))
			if a&7 == 2 { // ink red
				// must also have ink pixels
				for xb := byte(0); xb < 3; xb++ {
					if balPeek(emu, ula6(row*8+2, byte(col)+xb)) != 0 {
						redCells++
						break
					}
				}
			}
		}
	}
	if redCells == 0 {
		t.Fatal("no red-ink sprite pixels with hearts/diamonds in hand")
	}
	t.Logf("red ink cells=%d", redCells)

	savePNG(t, emu, filepath.Join(dir, "layout-play.png"))

	savePNG(t, emu, filepath.Join(dir, "layout-red.png"))

	// ---- (e) play a four-kind: played chip bars + joker plate ink live
	for i := 0; i < 4; i++ {
		emu.mem.Write(syms["HAND"]+uint16(i), byte(i*16+12))
	}
	emu.mem.Write(syms["HAND"]+4, byte(2*16+0))
	balRunFrames(emu, 3)
	for i := 0; i < 5; i++ {
		pressOnce(t, emu, syms, "ENTER")
		pressOnce(t, emu, syms, "6")
	}
	if n := balPeek(emu, syms["NSEL"]); n != 5 {
		t.Fatalf("NSEL=%d want 5", n)
	}
	pressOnce(t, emu, syms, "SPACE")
	for i := 0; i < 400 && balPeek(emu, syms["MODE"]) != bMODE_WON; i++ {
		balRunFrames(emu, 1)
	}
	if m := balPeek(emu, syms["MODE"]); m != bMODE_WON {
		t.Fatalf("MODE=%d want WON", m)
	}
	// settle again post-WON
	for i := 0; i < 200; i += 4 {
		h1 := hw()
		balRunFrames(emu, 4)
		if h1 == hw() {
			break
		}
		if i == 196 {
			t.Fatal("post-play paint never settled")
		}
	}
	// played chip bars: chip-bar pixels are the $FF solid block rows in
	// drawPlayed band; count solid rows across cols 16..23
	bars := 0
	for y := 0; y < 192; y++ {
		run := 0
		for x := 128; x < 192; x++ {
			if balPeek(emu, ula6(y, byte(x/8)))>>(7-uint8(x%8))&1 == 1 {
				run++
			} else {
				run = 0
			}
			if run >= 40 {
				bars++
				break
			}
		}
	}
	if bars == 0 {
		t.Fatal("no played chip-bar solid rows after play")
	}
	t.Logf("played chip-bar solid rows=%d", bars)
	// joker panel awarded at WON: jokers slot band near c12..c30 top rows 1-4
	jink := 0
	for row := 1; row <= 4; row++ {
		for col := 12; col < 32; col++ {
			for y := row * 8; y < row*8+8; y++ {
				if balPeek(emu, ula6(y, byte(col))) != 0 {
					jink++
				}
			}
		}
	}
	if jink < 20 {
		t.Fatalf("joker plate ink=%d want>=20 at WON", jink)
	}
	t.Logf("joker plate ink=%d", jink)
	savePNG(t, emu, filepath.Join(dir, "layout-score.png"))

}

// ula6: ULA pixel address for text-mode y (0..191) and x byte column.
func ula6(y int, xb byte) uint16 {
	return uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + int(xb))
}

// callIsolated jumps to sym with the halt-pad as return address and runs
// until PC lands on the pad ($BF00). Pushing curPC instead would continue
// executing the whole game after the routine returned — trashing RAM and
// the framebuffer and producing false-negative dumps.
func callIsolated(t *testing.T, emu *emulator, syms map[string]uint16, name string) {
	t.Helper()
	sp := emu.cpu.SP - 2
	emu.mem.Write(sp, 0x00)
	emu.mem.Write(sp+1, 0xBF)
	emu.cpu.SP = sp
	emu.cpu.PC = syms[name]
	n := 0
	for ; n < 400000 && emu.cpu.PC != 0xBF00; n++ {
		emu.cpu.StepInstruction()
	}
	if emu.cpu.PC != 0xBF00 {
		t.Fatalf("%s did not return (n=%d PC=%04X)", name, n, emu.cpu.PC)
	}
}
