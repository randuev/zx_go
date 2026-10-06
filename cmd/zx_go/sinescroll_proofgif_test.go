package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"testing"
)

// TestSinescrollProofGif — honest motion proof: boots the tap, runs 900
// frames, captures the DISPLAYED bank at every loop-top, deduplicates
// consecutive identical frames (the designed 2-frame holds under v5-B),
// and writes the changed frames to /tmp/ss_frames/fNNNN.png. Assembly by
// ffmpeg at -framerate 25 reproduces the true displayed cadence.
func TestSinescrollProofGif(t *testing.T) {
	syms := loadSinescrollSyms(t)
	tapPath := "/root/nerve-workspace/demos/sinescroll/sinescroll.tap"
	if p := os.Getenv("SINESCROLL_TAP"); p != "" {
		tapPath = p
	}
	emu := bootSinescroll(t, syms, tapPath)
	outDir := os.Getenv("SS_FRAMES_DIR")
	if outDir == "" {
		outDir = "/tmp/ss_frames"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	loop := syms["loop"]
	bbk := syms["bbk"]
	pal := [16][3]uint8{{0, 0, 0}, {0, 0, 215}, {215, 0, 0}, {215, 0, 215},
		{0, 215, 0}, {0, 215, 215}, {215, 215, 0}, {215, 215, 215},
		{0, 0, 255}, {0, 0, 255}, {255, 0, 0}, {255, 0, 255},
		{0, 255, 0}, {0, 255, 255}, {255, 255, 0}, {255, 255, 255}}
	rgb := make([]byte, 256*192*3)
	var prev [6144]byte
	havePrev := false
	written := 0
	for cyc := 0; cyc < 900; cyc++ {
		steps := 0
		for {
			steps++
			if steps > 400000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
			if emu.cpu.PC == loop && steps > 20 {
				break
			}
		}
		page := 10
		if emu.mem.Read(bbk)&8 != 0 {
			page = 14
		}
		g := emu.mem.RAM8KPage(page)[:6144]
		if havePrev {
			same := true
			for k := range prev {
				if prev[k] != g[k] {
					same = false
					break
				}
			}
			if same {
				continue
			}
		}
		copy(prev[:], g[:])
		havePrev = true
		// full attr-aware ULA decode -> RGB
		for y := 0; y < 192; y++ {
			pixelRow := y & 7
			charRow := y & 0x38
			third := y & 0xC0
			for x := 0; x < 256; x++ {
				pixelCol := x & 7
				charCol := x >> 3
				o := (pixelRow << 8) + (charRow << 2) + (third << 5) + charCol
				bit := byte(0x80) >> uint(pixelCol)
				col := uint8(7) // white ink
				if g[o]&bit == 0 {
					col = 0 // black paper (demo is pure B/W band)
				}
				i := (y*256 + x) * 3
				rgb[i], rgb[i+1], rgb[i+2] = pal[col][0], pal[col][1], pal[col][2]
			}
		}
		lvl := emu.mem.Read(syms["lvl"])
		name := filepath.Join(outDir, fmt.Sprintf("f%04d.png", written))
		lf, _ := os.OpenFile(filepath.Join(outDir, "lvls.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		fmt.Fprintf(lf, "%d %d\n", written, lvl)
		lf.Close()
		if err := writePNG(name, 256, 192, rgb); err != nil {
			t.Fatal(err)
		}
		hf := fnv.New64a()
		hf.Write(g)
		if written < 4 || cyc%150 == 0 {
			t.Logf("frame %d cyc%d bbk=%02X h=%016X", written, cyc, emu.mem.Read(bbk), hf.Sum64())
		}
		written++
	}
	fmt.Printf("proofgif frames=%d dir=%s\n", written, outDir)
}
