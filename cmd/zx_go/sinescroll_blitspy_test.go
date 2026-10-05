package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollBlitSpy — per-store forensic spy on the diag h8 blit.
// Ground truth: hook glyph-identity ($82E3 raw code), sprite-base ($831D),
// span2 stores ($83BD,$83C2) and span1 store ($8426); log, then diff the
// first glyphs against the font-derived expected raster at px0=8*cb+s.
func TestSinescrollBlitSpy(t *testing.T) {
	syms := map[string]uint16{}
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/diag/sinescroll_diag.sym")
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
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/diag/sinescroll_diag.tap")
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
	if code == nil {
		t.Fatal("no big CODE block")
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

	type rec struct {
		cb, gm, lvl, sfr, ylat, rawc byte
		sptr                          uint16
		stores                        [][3]uint16 // pc,dest,val
	}
	var glyphs []rec
	open := func() {
		glyphs = append(glyphs, rec{
			cb:   emu.mem.Read(syms["cb"]),
			gm:   emu.mem.Read(syms["gmask"]),
			lvl:  emu.mem.Read(syms["lvl"]),
			sfr:  emu.mem.Read(syms["sframe"]),
			ylat: emu.mem.Read(syms["ylat"]),
			sptr: uint16(emu.mem.Read(syms["sptr"])) | uint16(emu.mem.Read(syms["sptr"]+1))<<8,
		})
	}
	recStore := func(pc uint16) {
		if emu.mem.Read(syms["lvl"]) != 0 || len(glyphs) == 0 || len(glyphs) > 24 {
			return
		}
		g := &glyphs[len(glyphs)-1]
		g.stores = append(g.stores, [3]uint16{pc, emu.cpu.HL(), uint16(emu.cpu.A)})
	}
	var stageRaw byte
	emu.cpu.AddPreFetchHook("spyraw", func(pc uint16) {
		if pc == syms["gloop"]+0x0B { // byte AFTER DD 7E 00: A = raw code
			stageRaw = emu.cpu.A
		}
	})
	emu.cpu.AddPreFetchHook("spictx", func(pc uint16) {
		if pc == syms["g_body"] && len(glyphs) < 24 { // all globals final here
			open()
			glyphs[len(glyphs)-1].rawc = stageRaw
		}
	})
	emu.cpu.AddPreFetchHook("spys2a", func(pc uint16) { if pc == 0x83BD { recStore(pc) } })
	emu.cpu.AddPreFetchHook("spys2b", func(pc uint16) { if pc == 0x83C2 { recStore(pc) } })
	emu.cpu.AddPreFetchHook("spys1", func(pc uint16) { if pc == 0x8426 { recStore(pc) } })

	loop := syms["loop"]
	for f := 0; f < 3 && len(glyphs) == 0; f++ {
		steps := 0
		for {
			steps++
			if steps > 200000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
			if emu.cpu.PC == loop && steps > 20 && len(glyphs) > 0 {
				break
			}
		}
	}
	// glyph raw code -> char via CHARID+TEXT? raw code is TEXT byte itself.
	fmt.Printf("SPIED glyphs=%d (h8)\n", len(glyphs))
	fb := openFontRef()
	for i := range glyphs[:min(12, len(glyphs))] {
		g := &glyphs[i]
		ch := string(g.rawc)
		if g.rawc < 32 || g.rawc > 126 {
			ch = "?"
		}
		fmt.Printf("G%02d cb=%2d raw=%02X'%s' gmask=%02X s=%d ylat=%d sptr=%04X nstores=%d\n",
			i, g.cb, g.rawc, ch, g.gm, g.sfr, g.ylat, g.sptr, len(g.stores))
		// expected: 8 rows, pair (orig>>s at col cb, orig<<(8-s) at cb+1)
		if g.rawc >= 32 && g.rawc < 127 {
			for r := 0; r < 8; r++ {
				orig := fb[r*256+int(g.rawc)-32]
				lo := orig >> g.sfr
				hi := byte(0)
				if g.sfr != 0 {
					hi = orig << (8 - g.sfr)
				}
				fmt.Printf("    row%d exp cb=%02X@%d cb1=%02X@%d\n", r, lo, g.cb, hi, g.cb+1)
				if r*2 < len(g.stores) {
					fmt.Printf("    row%d act cb=%02X@%d cb1=%02X@%d\n",
						r, g.stores[r*2][2], g.stores[r*2][1]&31, g.stores[r*2+1][2], g.stores[r*2+1][1]&31)
				}
			}
		}
	}
}

func openFontRef() []byte {
	b, _ := os.ReadFile("/root/nerve-workspace/demos/snow/font.bin")
	return b
}
