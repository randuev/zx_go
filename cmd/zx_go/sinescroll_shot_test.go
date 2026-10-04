package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollShot — LOOK at the screen. Every gate measures deltas and
// none of them can read text. This dumps the DISPLAYED page as ASCII art
// (half-res, 2px per char) at fixed intervals and writes PNGs, so we can
// literally see what Seva sees: is it one readable band? two bands? dots?
func TestSinescrollShot(t *testing.T) {
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	if err != nil {
		t.Fatalf("read sym: %v", err)
	}
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "EQU 0x") {
			if v, e := strconv.ParseUint(rest[6:], 16, 16); e == nil {
				syms[name] = uint16(v)
			}
		}
	}
	tapPath := "/root/nerve-workspace/demos/sinescroll/sinescroll.tap"
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := parseTap(raw)
	if len(blocks) != 1 {
		t.Fatalf("want 1 CODE block, got %d", len(blocks))
	}
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)

	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFFFE, 0x00)
	emu.mem.Write(0xFFFF, 0x00)
	emu.cpu.PC = base

	litRunes := func(page []byte) []string {
		// 192 pixel rows -> ASCII rows; each char = 2 px (both lit, one lit, none)
		out := make([]string, 0, 192)
		for y := 0; y < 192; y++ {
			row := strings.Builder{}
			for xb := 0; xb < 32; xb++ {
				v := page[((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5)+xb]
				for hp := 0; hp < 4; hp++ {
					s := ""
					for px := 0; px < 2; px++ {
						x := xb*8 + hp*2 + px
						if v&(0x80>>(x&7)) != 0 {
							s += "#"
						} else {
							s += "."
						}
					}
					switch s {
					case "##":
						row.WriteString("█")
					case "#.":
						row.WriteString("▌")
					case ".#":
						row.WriteString("▐")
					default:
						row.WriteString(" ")
					}
				}
			}
			if strings.TrimSpace(row.String()) != "" {
				out = append(out, fmt.Sprintf("y%3d %s", y, row.String()))
			}
		}
		return out
	}

	prevLvl := byte(0)
	for frame := 0; frame < 400; frame++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if frame < 60 { // let initial paint/erase settle out of the shots
			continue
		}
		p := int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
		lvl := emu.mem.Read(syms["lvl"])
		page := emu.mem.RAM8KPage(10)
		if emu.mem.Read(syms["bbk"])&0x08 != 0 {
			page = emu.mem.RAM8KPage(14)
		}
		// ink census per row: the two-band fingerprint
		litRows := func() (int, []int) {
			n := 0
			var rows []int
			for y := 0; y < 192; y++ {
				c := 0
				for xb := 0; xb < 32; xb++ {
					c += bits8(page[((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5)+xb])
				}
				if c > 0 {
					n += c
					rows = append(rows, y)
				}
			}
			return n, rows
		}
		watch := lvl != prevLvl // every transition gets full dumps
		prevLvl = lvl
		if !watch && frame%40 != 0 {
			continue
		}
		n, rows := litRows()
		fmt.Printf("\n=== f%d p=%d lvl%d bbk=%02X ink=%d y=%d..%d ===\n",
			frame, p, lvl, emu.mem.Read(syms["bbk"]), n, firstOf(rows), lastOf(rows))
		// ATTR bytes of the display bank (offset $1800 + row*32): non-zero
		// attrs in glyph rows = ink misrouted into attributes (the colored
		// speckles Seva sees on hardware).
		for r := 8; r <= 21; r++ {
			line := page[0x1800+r*32 : 0x1800+r*32+32]
			nz := 0
			var s strings.Builder
			for _, b := range line {
				if b != 0 {
					nz++
					fmt.Fprintf(&s, "%02X ", b)
				}
			}
			if nz > 0 {
				fmt.Printf("  ATTR r%d nz=%d %s\n", r, nz, s.String())
			}
		}
		for _, r := range litRunes(page) {
			fmt.Println(r)
		}
		// PNG every shot at transitions, every 80 frames otherwise
		if watch || frame%80 == 0 {
			rgb := make([]byte, 256*192*3)
			for y := 0; y < 192; y++ {
				for x := 0; x < 256; x++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
					i := (y*256 + x) * 3
					lit := page[o]&(0x80>>(x&7)) != 0
					if lit {
						rgb[i], rgb[i+1], rgb[i+2] = 0xFF, 0xFF, 0xFF
					}
				}
			}
			fn := fmt.Sprintf("/tmp/sine_shot_f%d.png", frame)
			if err := writePNG(fn, 256, 192, rgb); err != nil {
				t.Fatalf("png: %v", err)
			}
		}
	}
}

func firstOf(v []int) int {
	if len(v) == 0 {
		return -1
	}
	return v[0]
}
func lastOf(v []int) int {
	if len(v) == 0 {
		return -1
	}
	return v[len(v)-1]
}
func bits8(v byte) int {
	c := 0
	for v != 0 {
		c += int(v & 1)
		v >>= 1
	}
	return c
}
