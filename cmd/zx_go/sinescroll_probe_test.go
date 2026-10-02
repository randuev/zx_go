package main

// sinescroll_probe_test.go — targeted forensic: at a stable parked frame,
// replay walk()'s DRAW-pass math from the demo's live globals and diff the
// predicted ink bytes against actual screen RAM. First divergences localize
// pointer/geometry bugs precisely. Scratch harness, delete after use.

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSinescrollProbe(t *testing.T) {
	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			rest = strings.TrimSpace(rest)
			if !strings.HasPrefix(rest, "EQU 0x") {
				continue
			}
			if v, err := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); err == nil {
				syms[name] = uint16(v)
			}
		}
	}
	sym := func(n string) uint16 { return syms[n] }

	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	if err != nil {
		t.Fatal(err)
	}
	// one CODE block
	var pend uint16
	off := 0
	var baddr uint16
	var bdata []byte
	for off+2 <= len(raw) {
		n := int(raw[off]) | int(raw[off+1])<<8
		off += 2
		if off+n > len(raw) {
			break
		}
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0 && p[1] == 3 {
			pend = uint16(p[14]) | uint16(p[15])<<8
			continue
		}
		if p[0] == 0xFF && pend != 0 {
			baddr = pend
			bdata = p[1 : n-1]
		}
	}
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bdata {
		emu.mem.Write(baddr+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFFFE, 0x00)
	emu.mem.Write(0xFFFF, 0x00)
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	ram := func(a uint16) byte { return emu.mem.Read(a) }
	rdw := func(a uint16) uint16 { return uint16(emu.mem.Read(a)) | uint16(emu.mem.Read(a+1))<<8 }

	halt := sym("lhalt")
	// run until parked, lvl==0, and at least frame 120 (mid h8 dwell)
	for i := 0; i < 400; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.cpu.PC == halt+1 && ram(sym("lvl")) == 0 && i > 100 {
			t.Logf("parked at f%d", i)
			break
		}
	}
	// committed state at park: op/olvl = last DRAWN geometry. Recompute.
	p := rdw(sym("p"))
	op := rdw(sym("op"))
	lvl := ram(sym("lvl"))
	olvl := ram(sym("olvl"))
	hh := ram(sym("hh"))
	nbb := ram(sym("nbb"))
	half := ram(sym("half"))
	lptrb := ram(sym("lptrb"))
	cb := ram(sym("cb"))
	t.Logf("globals: p=%d op=%d lvl=%d olvl=%d hh=%d nbb=%d half=%d lptrb=$%02X cb=%d", p, op, lvl, olvl, hh, nbb, half, lptrb, int(int8(cb)))

	// Predict DRAW pass (geometry = op/olvl, ink state: committed screen has
	// draw applied at op? No: at park, last walk was DRAW at p (then op=p).
	// So predicted ink = walk(op, olvl) with paint=$FF.
	pbyte := op >> 3
	q := pbyte >> olvl
	nb := uint16(1) << olvl
	cb0 := q*nb - pbyte
	t.Logf("predict: pbyte=%d q=%d cb0=%d", pbyte, q, int(int8(cb0)))
	yoff := func(x uint16) byte { return ram(sym("YOFF") + x) }
	deftab := func(y uint16) uint16 { return rdw(sym("DEFTAB") + y*2) }
	screen := emu.mem.RAM8KPage(10)

	type miss struct {
		x, y     int
		exp, got byte
		srcAddr  uint16
	}
	misses := []miss{}
	total, painted := 0, 0
	for g := 0; g < 32; g++ {
		cbb := int(int8(cb0)) + g*int(nb)
		if cbb >= 32 {
			break
		}
		rawc := ram(sym("TEXT") + q + uint16(g))
		ci := ram(sym("CHARID") + uint16(rawc))
		if ci == 0xFF {
			continue
		}
		// sprite src = ptr word in CHARPTR{8C00|lptrb} + ci*2
		sptr := rdw(0x8C00 + uint16(lptrb) + uint16(ci)*2)
		x := cbb*8 + int(half)
		if x > 255 {
			x = 0
		}
		cy := int(yoff(uint16(x)))
		ytop := cy - int(half)
		for r := 0; r < int(hh); r++ {
			sb := ram(sptr + uint16(r)*nb)
			if sb == 0 {
				continue
			}
			addr := deftab(uint16(ytop+r)) + uint16(cbb)
			painted++
			off := addr - 0x4000
			if off >= 0x4000 {
				continue
			}
			total++
			got := screen[off]
			if got&sb != sb {
				misses = append(misses, miss{(cbb) * 8, ytop + r, sb, got, addr})
			}
		}
	}
	t.Logf("predict: sprite-ink writes=%d verified=%d misses=%d", painted, total, len(misses))
	for i, m := range misses[:12] {
		t.Logf("MISS[%d] addr=$%04X (x=%d y=%d bytecol=%d) exp=%08b got=%08b", i, m.srcAddr+0, m.x, m.y, (m.srcAddr-0x4000)&31, m.exp, m.got)
	}
	// Also dump one row around the baseline from screen + predicted columns
	t.Logf("screen row-scan baseline: chars seen at op:")
	for g := 0; g < 12; g++ {
		t.Logf("  g%d raw=%q ci=%d", g, string(ram(sym("TEXT")+q+uint16(g))), ram(sym("CHARID")+uint16(ram(sym("TEXT")+q+uint16(g)))))
	}
}

// Isolate: park, snapshot screen, run walk() live (as DRAW pass) by pushing
// a return trap, byte-diff predicted-ink vs screen-after.
