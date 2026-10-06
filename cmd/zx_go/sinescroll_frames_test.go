package main

import (
	"strings"
	"strconv"

	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollFrames — captures consecutive DISPLAYED-bank snapshots of
// the v4.1 scroller to answer: is the screen actually cleared between frames?
func TestSinescrollFrames(t *testing.T) {
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks := parseTap(raw)
	var base uint16
	var code []byte
	for _, b := range blocks {
		if d, ok := b[1].([]byte); ok && len(d) > 13000 {
			base = b[0].(uint16)
			code = d
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
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
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = base
	loop := uint16(0x8114) // v6.6c default; prefer symbol below
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			rest = strings.TrimSpace(rest)
			if name == "loop" && strings.HasPrefix(rest, "EQU 0x") {
				if v, e := strconv.ParseUint(rest[6:], 16, 16); e == nil {
					loop = uint16(v)
				}
			}
		}
	}

	frameGrid := func(page int) [][]int {
		b := emu.mem.RAM8KPage(page)
		g := make([][]int, 192)
		for y := 0; y < 192; y++ {
			g[y] = make([]int, 256)
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			for xb := 0; xb < 32; xb++ {
				v := b[o+xb]
				for bt := 0; bt < 8; bt++ {
					g[y][xb*8+bt] = int((v >> (7 - bt)) & 1)
				}
			}
		}
		return g
	}
	ink := func(g [][]int) int {
		n := 0
		for y := range g {
			for x := range g[y] {
				n += g[y][x]
			}
		}
		return n
	}
	// run to frame 100 (scrolled well in), then capture 6 consecutive frames.
	frames := 0
	capN := 6
	grids := make([][][]int, 0, capN)
	pVals := make([]uint16, 0, capN)
	for frames < 600 {
		steps := 0
		for {
			steps++
			if steps > 300000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
			if emu.cpu.PC == loop && steps > 20 {
				break
			}
		}
		frames++
		if frames < 50 {
			continue
		}
		// read p + which bank is displayed
		p := uint16(emu.mem.Read(0x8E00)) | uint16(emu.mem.Read(0x8E01))<<8
		bbk := emu.mem.Read(0x8E21)
		_ = bbk
				g5 := frameGrid(10)
		g7 := frameGrid(14)
		fmt.Printf("FRAME p=%d ink_b5=%d ink_b7=%d\n", p, ink(g5), ink(g7))
		if len(grids) < capN {
			grids = append(grids, g5)
			pVals = append(pVals, p)
		}
		if len(grids) == capN {
			frames = 1000 // exit outer too
			break
		}
	}
	// ghost check: ink in frame k at positions NOT in frame k+1 = trailing?
	for i := 0; i+1 < len(grids); i++ {
		a, b := grids[i], grids[i+1]
		ghost := 0
		for y := range a {
			for x := range a[y] {
				if a[y][x] == 1 && b[y][x] == 0 {
					ghost++
				}
			}
		}
		fmt.Printf("f%d->f%d ink %d->%d cleared=%d added=%d\n", i, i+1, ink(a), ink(b), ghost, ink(b)-ink(a)+ghost)
	}
	// save stacked PNG of captured frames at 3x
	sep := func() {}
	sep()
	var rawb []byte
	Z := 3
	for i, g := range grids {
		_ = i
		for y := 0; y < 192; y++ {
			for zy := 0; zy < Z; zy++ {
				rawb = append(rawb, 0)
				for x := 0; x < 256; x++ {
					pix := []byte{0, 0, 0}
					if g[y][x] == 1 {
						pix = []byte{0x66, 0xff, 0x66}
					}
					for zz := 0; zz < Z; zz++ {
						rawb = append(rawb, pix...)
					}
				}
			}
		}
	}
	var body bytes.Buffer
	zw := zlib.NewWriter(&body)
	zw.Write(rawb)
	zw.Close()
	chunk := func(typ string, d []byte) []byte {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, uint32(len(d)))
		b.WriteString(typ)
		b.Write(d)
		binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), d...)))
		return b.Bytes()
	}
	var png bytes.Buffer
	png.Write([]byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10})
	ihdr := new(bytes.Buffer)
	binary.Write(ihdr, binary.BigEndian, uint32(256*Z))
	binary.Write(ihdr, binary.BigEndian, uint32(192*Z*len(grids)))
	ihdr.Write([]byte{8, 2, 0, 0, 0})
	png.Write(chunk("IHDR", ihdr.Bytes()))
	png.Write(chunk("IDAT", body.Bytes()))
	png.Write(chunk("IEND", nil))
	os.WriteFile("/tmp/sinescroll_frames.png", png.Bytes(), 0o644)
}
