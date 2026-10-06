package main

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// h16 motion probe: per-frame displayed centroid during h16 dwell.
// Question: does h16 text actually step every paint, or does it stall
// on some paints (the STILLDIAG identical flips)?
func TestTrailPanelH16MotionProbe(t *testing.T) {
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
	lvlA := syms["lvl"]
	pA := syms["p"]
		inH16 := false
	dwells := 0
	for f := 0; f < 3000; f++ {
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
		lvl := emu.mem.Read(lvlA)
		p := uint16(emu.mem.Read(pA)) | uint16(emu.mem.Read(pA+1))<<8
		bbk := emu.mem.Read(syms["bbk"])
		page := int(emu.mem.ScreenPage)
		chip := emu.mem.GetPage(page)[:6144]
		lit, cxsum := 0, 0
		for y := 0; y < 192; y++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			for xb := 0; xb < 32; xb++ {
				v := chip[o+xb]
				for b := 0; b < 8; b++ {
					if v&(0x80>>b) != 0 {
						lit++
						cxsum += xb*8 + (7 - b)
					}
				}
			}
		}
		cx := -1
		if lit > 0 {
			cx = cxsum / lit
		}
		hf, _ := fnv32(chip)
		if lvl == 1 {
			if !inH16 {
				inH16 = true
				dwells++
				if dwells > 2 {
					return
				}
			}
			fmt.Printf("H16f f%d p%d q%d bbk=%02X lit%d cx%d h=%08X\n",
				f, p, p>>4, bbk, lit, cx, hf)
		} else {
			inH16 = false
		}
	}
}

func fnv32(b []byte) (uint32, error) {
	h := fnv.New32a()
	if _, err := h.Write(b); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}
var _ = bytes.MinRead
