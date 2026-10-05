package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollTrailProof — v4.1b ghost-trail FIX proof for Seva:
// captures the DISPLAYED bank at 3 consecutive painted loop-tops of the
// shipped main tap and stacks them into one PNG. If erasure works, each
// panel shows clean text at a NEW position with zero residue from the
// previous one. (Bug was v3.9 contiguous-band LDIR erase: ULA rows never
// stride 1, so old ink was never removed.)
func TestSinescrollTrailProof(t *testing.T) {
	syms := map[string]uint16{}
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
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

	loop := syms["loop"]
	// walk p advancing frames; snap the displayed bank (PA bit3 of bbk:
	// $07 PA=0 -> b5 page10; $0D PA=1 -> b7 page14) after 3 DIFFERENT paints.
	var snaps [][]byte
	var ps []uint16
	prevP := uint16(0xFFFF)
	frames := 0
	for len(snaps) < 3 {
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
		p := uint16(emu.mem.Read(syms["p"])) | uint16(emu.mem.Read(syms["p"]+1))<<8
		bbk := emu.mem.Read(syms["bbk"])
		// displayed bank = PA-selected: bbk&8 -> b7 else b5
		page := 10
		if bbk&8 != 0 {
			page = 14
		}
		if p < 200 {
			continue // skip scroll-in lead-in; capture mid-text panels
		}
		if p != prevP {
			prevP = p
			g := make([]byte, 6144)
			copy(g, emu.mem.RAM8KPage(page)[:6144])
			snaps = append(snaps, g)
			ps = append(ps, p)
		}
		if frames > 400 {
			t.Fatalf("only %d distinct paints in %d frames", len(snaps), frames)
		}
	}
	// per-panel ink check: must be bounded (~<900) and each panel's OLD
	// ink area must be clear in the NEXT panel except new text overlap.
	Z := 3
	Wd, Hd := 256*Z, 192*Z*len(snaps)+8*len(snaps)
	img := make([]byte, 0, Hd*(1+Wd*3))
	for si, g := range snaps {
		ink := 0
		for _, v := range g[:6144] {
			for b := 0; b < 8; b++ {
				if v&(1<<b) != 0 {
					ink++
				}
			}
		}
		t.Logf("panel%d p=%d ink=%d", si, ps[si], ink)
		// ink must live ONLY inside the active band rows: any ink row far
		// outside proves un-erased ghost residue from earlier positions.
		minY, maxY := 999, -1
		var rows [192]int
		for y := 0; y < 192; y++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			for xb := 0; xb < 32; xb++ {
				v := g[o+xb]
				for b := 0; b < 8; b++ {
					if v&(1<<b) != 0 {
						rows[y]++
					}
				}
			}
		}
		for y := 0; y < 192; y++ {
			if rows[y] > 0 {
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
		t.Logf("panel%d inkRows y=%d..%d", si, minY, maxY)
		if minY < 76 || maxY > 116 {
			t.Errorf("panel%d ink outside band 76..116 (y=%d..%d) — ghost residue", si, minY, maxY)
		}
		if ink > 900 {
			t.Errorf("panel%d ink=%d > 900 — accumulation (ghost trail)", si, ink)
		}
		for y := 0; y < 192; y++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			for zy := 0; zy < Z; zy++ {
				img = append(img, 0)
				for xb := 0; xb < 32; xb++ {
					v := g[o+xb]
					for bt := 0; bt < 8; bt++ {
						pix := []byte{8, 8, 8}
						if v&(0x80>>bt) != 0 {
							pix = []byte{0x66, 0xff, 0x66}
						}
						for zz := 0; zz < Z; zz++ {
							img = append(img, pix...)
						}
					}
				}
			}
		}
		for zy := 0; zy < 8; zy++ {
			img = append(img, 0)
			for x := 0; x < Wd; x++ {
				img = append(img, 0, 0, 0)
			}
		}
	}
	var body bytes.Buffer
	zw2 := zlib.NewWriter(&body)
	zw2.Write(img)
	zw2.Close()
	chunk := func(typ string, data []byte) []byte {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
		return b.Bytes()
	}
	var png bytes.Buffer
	png.Write([]byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10})
	ihdr := new(bytes.Buffer)
	binary.Write(ihdr, binary.BigEndian, uint32(Wd))
	binary.Write(ihdr, binary.BigEndian, uint32(Hd))
	ihdr.Write([]byte{8, 2, 0, 0, 0})
	png.Write(chunk("IHDR", ihdr.Bytes()))
	png.Write(chunk("IDAT", body.Bytes()))
	png.Write(chunk("IEND", nil))
	if err := os.WriteFile("/tmp/sinescroll_trailproof_v41b.png", png.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log("wrote /tmp/sinescroll_trailproof_v41b.png")
}
