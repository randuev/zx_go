package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollMidpaint reproduces SEVA's CAMERA VIEW of the kiosk tape:
// a frame captured WHILE the paint front is sweeping. The kiosk paints the
// DISPLAYED bank directly ($4xxx, zero paging — its whole point) and the
// paint costs ~2 frames, so his TV shows the half-painted band: smeared
// glyph fragments where the beam is, old content above, blank below. My
// after-the-fact screenshots can NEVER show this. Composite: rows the beam
// already passed = CURRENT RAM (fresh paint), rows below = PRE-paint copy.
func TestSinescrollMidpaint(t *testing.T) {
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/diag/sinescroll_diag.sym")
	if err != nil {
		t.Fatal(err)
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
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/diag/code_diag.tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks := parseTap(raw)
	base := blocks[0][0].(uint16)
	code := blocks[0][1].([]byte)
	for _, b := range blocks[1:] {
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
	emu.mem.Write(0xFFFE, 0)
	emu.mem.Write(0xFFFF, 0)
	emu.cpu.PC = base

	loop := syms["loop"]
	walk := syms["walk"]
	bandsave := syms["bandsave"]
	// display bank first 8K raw (rows 0..127 — the sine band is inside)
	saveBank := func() []byte {
		p := emu.mem.RAM8KPage(10)
		if p == nil {
			t.Fatal("no page10")
		}
		b := make([]byte, 8192)
		copy(b, p)
		return b
	}
	capture := func(tag string, old []byte) {
		line, _ := emu.ula.BeamPosition()
		shown := line - 64 // rows already swept (top blank ~64 lines of 311)
		rgb := make([]byte, 256*192*3)
		for y := 0; y < 192; y++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			for xb := 0; xb < 32; xb++ {
				var v byte
				if y < shown {
					v = emu.mem.Read(uint16(0x4000 + o + xb))
				} else {
					v = old[o+xb]
				}
				for bt := 0; bt < 8; bt++ {
					i := (y*256 + xb*8 + bt) * 3
					if v&(0x80>>bt) != 0 {
						rgb[i], rgb[i+1], rgb[i+2] = 0xFF, 0xFF, 0xFF
					}
				}
			}
		}
		if err := writePNG("/tmp/sine_kiosk_mid_"+tag+".png", 256, 192, rgb); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("MIDPAINT %s beamLine=%d shown=%d pc=$%04X\n", tag, line, shown, emu.cpu.PC)
	}

	// Phase 1: idle to the last clean loop head (band still blank-ish), copy old.
	for f := 0; f < 400; f++ {
		steps := 0
		for {
			steps++
			if emu.cpu.PC == loop && steps > 20 {
				break
			}
			if steps > 300000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
		}
		if emu.mem.Read(syms["diagf"]) == 0 {
			// we are at a clean loop head BEFORE the first paint frame
			break
		}
	}
	old := saveBank()
	// Phase 1.5: forensics — every bank-5 (display) byte write during the
	// first paint frame, with writer PC. Answers WHERE the glyph bytes go.
	type wrec struct {
		pc   uint16
		addr uint16
		val  byte
		bank int
	}
	var ws []wrec
	var samples []wrec
	bankhits := map[int]int{}
	hook := func(bank int, addr uint16, val byte) {
		// hook reports physical bank + OFFSET within the 16K bank;
		// display $4000..$57FF == offsets 0x0000..0x17FF
		if addr < 0x1800 && (bank == 5 || bank == 7) {
			bankhits[bank]++
			ws = append(ws, wrec{emu.cpu.PC, addr, val, bank})
			if len(samples) < 8 {
				samples = append(samples, wrec{emu.cpu.PC, addr, val, bank})
			}
		}
	}
	procSpy := syms["proc"]
	procHits := 0
	emu.cpu.AddPreFetchHook("procspy", func(pc uint16) {
		if pc == procSpy {
			procHits++
		}
	})
	gl := syms["gloop"]
	var glog []string
	emu.cpu.AddPreFetchHook("gloopspy", func(pc uint16) {
		if pc == gl {
			glog = append(glog, fmt.Sprintf("cb=%02X gmask=%d hh=%d p=%04X lvl=%d",
				emu.mem.Read(syms["cb"]), emu.mem.Read(syms["gmask"]),
				emu.mem.Read(syms["hh"]),
				uint16(emu.mem.Read(syms["p"]))|uint16(emu.mem.Read(syms["p"]+1))<<8,
				emu.mem.Read(syms["lvl"])))
		}
	})
	emu.mem.SetRAMWriteHook(hook)
	for f := 0; f < 400; f++ {
		steps := 0
		for {
			steps++
			if emu.cpu.PC == loop && steps > 20 {
				break
			}
			if steps > 300000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
		}
		if emu.mem.Read(syms["diagf"]) == 1 {
			break
		}
	}
	emu.mem.SetRAMWriteHook(nil)
	span2, span1 := 0, 0
	var prevAddr uint16
	havePrev := false
	for _, w := range ws {
		if havePrev && (w.addr&0xFF) == ((prevAddr&0xFF)+1)%256 && (w.addr&0xFE00) == (prevAddr & 0xFE00) {
			span2++
		} else {
			span1++
		}
		prevAddr = w.addr
		havePrev = true
	}
	fmt.Printf("gloop visits: %d; span2-successions=%d singletons=%d\n", len(glog), span2, span1)
	for i, s := range glog {
		if i < 6 || i >= len(glog)-4 {
			fmt.Println(" ", s)
		}
	}
	fmt.Printf("HOOK: %d writes to $4000-$57FF; banks touched: %v; bbk=%02X\n",
		len(ws), bankhits, emu.mem.Read(syms["bbk"]))
	{
		f, err := os.Create("/tmp/bank5_band.txt")
		if err == nil {
			p := emu.mem.RAM8KPage(10)
			for off := 0; off < 0x1800; off += 32 {
				fmt.Fprintf(f, "%04X", off)
				for x := 0; x < 32; x++ {
					fmt.Fprintf(f, " %02X", p[off+x])
				}
				fmt.Fprintln(f)
			}
			f.Close()
		}
	}
	{
		f, err := os.Create("/tmp/band_writes.txt")
		if err == nil {
			for _, w := range ws {
				fmt.Fprintf(f, "%04X %02X %04X\n", w.addr, w.val, w.pc)
			}
			f.Close()
		}
	}
	for _, s := range samples {
		fmt.Printf("  sample bank=%d addr=$%04X val=%02X pc=$%04X\n", s.bank, s.addr, s.val, s.pc)
	}
	// per writer-PC histogram
	pch := map[uint16]int{}
	colcnt := map[uint16]int{} // byte-col -> nonzero writes
	for _, w := range ws {
		pch[w.pc]++
		if w.val != 0 && w.bank == 5 {
			a := w.addr // bank offset == $4000-relative
			third := (a >> 11) & 3
			prow := (a >> 8) & 7
			crow := (a >> 5) & 7
			xcol := a & 31
			y := third*64 + crow*8 + prow
			if y >= 84 && y <= 107 {
				colcnt[xcol]++
			}
		}
	}
	fmt.Println("band lit-byte writes per byte-column (nonzero):")
	for c := 0; c < 32; c++ {
		if colcnt[uint16(c)] > 0 {
			fmt.Printf(" col%02d=%d", c, colcnt[uint16(c)])
		}
	}
	fmt.Println()
	allcol := map[uint16]int{}
	for _, w := range ws {
		allcol[w.addr&31]++
	}
	fmt.Print("ALL-write cols (nonzero): ")
	for c := 0; c < 32; c++ {
		if allcol[uint16(c)] > 0 {
			fmt.Printf(" %d:%d", c, allcol[uint16(c)])
		}
	}
	fmt.Println()
	// top writer PCs
	fmt.Println("top writer PCs:")
	type kv struct {
		k uint16
		v int
	}
	var ks []kv
	for k, v := range pch {
		ks = append(ks, kv{k, v})
	}
	for i := 0; i < len(ks); i++ {
		for j := i + 1; j < len(ks); j++ {
			if ks[j].v > ks[i].v {
				ks[i], ks[j] = ks[j], ks[i]
			}
		}
	}
	for i := 0; i < len(ks) && i < 10; i++ {
		nm := "?"
		for sname, sa := range syms {
			if sa == ks[i].k {
				nm = sname
			}
		}
		fmt.Printf("  $%04X %-10s x%d\n", ks[i].k, nm, ks[i].v)
	}

	// Phase 2: run INTO the first paint; capture while walk is drawing,
	// at raster rows 50/100/150 (25%/50%/75% of the screen swept). This is
	// precisely what a camera photo of the CRT catches.
	done := map[string]bool{}
	marks := []int{50, 100, 150}
	for s := 0; s < 900000; s++ {
		emu.cpu.StepInstructionWithIRQ()
		pc := emu.cpu.PC
		if emu.mem.Read(syms["diagf"]) == 1 {
			if !done["complete"] {
				capture("complete", old)
				done["complete"] = true
				// ground truth: ASCII of live display RAM right now, same path
				// as TestSinescrollDiag's PASS dump
				for y := 84; y < 108; y++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
					line := ""
					for xb := 0; xb < 32; xb++ {
						v := emu.mem.Read(uint16(0x4000 + o + xb))
						if v == 0 {
							line += " "
						} else {
							line += "#"
						}
					}
					fmt.Printf("LIVE y=%3d |%s|\n", y, line)
				}
			}
			break
		}
		if pc >= walk && pc < bandsave {
			line, _ := emu.ula.BeamPosition()
			shown := line - 64
			for _, m := range marks {
				tag := fmt.Sprintf("row%03d", m)
				if !done[tag] && shown >= m {
					capture(tag, old)
					done[tag] = true
				}
			}
		}
	}
	fmt.Println("captured:", done)
}

// TestSinescrollBakeIntegrity verifies $9600..$B4C0 bake block landed intact
// in RAM as shipped inside the diag tap (the midpaint fragment hunt).
func TestSinescrollBakeIntegrity(t *testing.T) {
	bake, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/bake.bin")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/diag/code_diag.tap")
	if err != nil {
		t.Fatal(err)
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
	blocks := parseTap(raw)
	var code []byte
	var base uint16
	for _, b := range blocks {
		if d, ok := b[1].([]byte); ok && len(d) > 13000 {
			base = b[0].(uint16)
			code = d
		}
	}
	if code == nil {
		t.Fatal("no big block")
	}
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	mism := 0
	first := -1
	for i := 0; i < len(bake); i++ {
		got := emu.mem.Read(uint16(0x9600+i))
		if got != bake[i] {
			mism++
			if first < 0 {
				first = i
			}
		}
	}
	if first >= 0 {
		fmt.Printf("BAKE: len=%d mismatches=%d firstAt=$%04X exp=%02X got=%02X\n", len(bake), mism, 0x9600+first, bake[first], emu.mem.Read(uint16(0x9600+first)))
	} else {
		fmt.Printf("BAKE: len=%d intact\n", len(bake))
	}
	bad := 0
	for ci := 0; ci < 38; ci++ {
		w := uint16(emu.mem.Read(uint16(0x8C00+ci*2))) | uint16(emu.mem.Read(uint16(0x8C01+ci*2)))<<8
		if w != 0xA1C0+uint16(ci)*128 {
			bad++
			fmt.Printf("CHARPTR8[%d]=$%04X expected $%04X\n", ci, w, 0xA1C0+uint16(ci)*128)
		}
	}
	fmt.Println("CHARPTR8 mismatches:", bad)
}
