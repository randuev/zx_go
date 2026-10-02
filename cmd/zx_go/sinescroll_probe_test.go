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
func TestSinescrollWalkIsolate(t *testing.T) {
	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			if rest = strings.TrimSpace(rest); strings.HasPrefix(rest, "EQU 0x") {
				if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
					syms[name] = uint16(v)
				}
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
	raw, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	var bdata []byte
	var baddr uint16
	off := 0
	var pend uint16
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
			baddr, bdata = pend, p[1:n-1]
		}
	}
	emu, _ := newEmulator(roms.ModelPlus2)
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bdata {
		emu.mem.Write(baddr+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000
	emu.cpu.IM = 1
	halt := sym("lhalt")
	// CHARID immediately after load:
	t.Logf("CHARID post-load: $9AA0=$%02X w=$%02X", emu.mem.Read(0x9AA0), emu.mem.Read(0x9A80+0x77))
	for i := 0; i < 400; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.cpu.PC == halt+1 && emu.mem.Read(sym("lvl")) == 0 && i > 100 {
			t.Logf("parked f%d", i)
			break
		}
	}
	ram := func(a uint16) byte { return emu.mem.Read(a) }
	rdw := func(a uint16) uint16 { return uint16(emu.mem.Read(a)) | uint16(emu.mem.Read(a+1))<<8 }
	// globals right now (post-draw park): p/op equal, lvl/olvl equal, paint should be $FF
	t.Logf("globals: p=%d op=%d lvl=%d olvl=%d hh=%d nbb=%d half=%d lptrb=$%02X paint+1=$%02X wlvl=%d wp=%d",
		rdw(sym("p")), rdw(sym("op")), ram(sym("lvl")), ram(sym("olvl")), ram(sym("hh")), ram(sym("nbb")), ram(sym("half")), ram(sym("lptrb")), ram(sym("paint")+1), ram(sym("wlvl")), rdw(sym("wp")))
	before := append([]byte(nil), emu.mem.RAM8KPage(10)[:0x4000]...)
	// run walk as draw pass: wlvl/lvl=0,hh=8,nbb=1; paint+1 currently? ensure $FF
	emu.mem.Write(sym("paint")+1, 0xFF)
	// push trap ret address = halt (PC lands on halt after walk ret)
	sp := emu.cpu.SP - 2
	emu.mem.Write(sp, byte(halt))
	emu.mem.Write(sp+1, byte(halt>>8))
	emu.cpu.SP = sp
	emu.cpu.PC = sym("walk")
	emu.cpu.Halted = false
	steps := 0
	rsnap := 0
	micro := 0
	microOn := false
	type regline struct{ pc, af, bc, de, hl, ix, iy uint16 }
	var regs []regline
	type wr struct{ a, de, hl uint16 }
	var writes []wr
	type dec struct{ ix, raw, ci uint16 }
	var decs []dec
	seq := []uint16{}
	for emu.cpu.PC != halt+1 && steps < 200000 {
		seq = append(seq, emu.cpu.PC)
		if rsnap < 40 {
			rsnap++
			regs = append(regs, regline{emu.cpu.PC, uint16(emu.cpu.A)<<8 | uint16(emu.cpu.F), emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL(), emu.cpu.IX, emu.cpu.IY})
		}
		if emu.cpu.PC == 0x81B8 { // gloop entry marker region: count glyph iterations
			micro++
		}
		if micro == 3 {
			microOn = true
		}
		if microOn && micro == 4 {
			microOn = false
		}
		if microOn && micro <= 4 {
			t.Logf("MS $%04X af=%02X%02X hl=%04X de=%04X bc=%04X ix=%04X iy=%04X sp=%04X", emu.cpu.PC, emu.cpu.A, emu.cpu.F, emu.cpu.HL(), emu.cpu.DE(), emu.cpu.BC(), emu.cpu.IX, emu.cpu.IY, emu.cpu.SP)
		}
		if emu.cpu.PC == 0x81C0 { // jp nc,w_done — A=(cb)
			t.Logf("CBDEC cb=%d flags_c=%d ix=$%04X raw=%q", emu.cpu.A, func() int {
				if emu.cpu.F&1 != 0 {
					return 1
				}
				return 0
			}(), emu.cpu.IX, string(rune(emu.mem.Read(emu.cpu.IX))))
			if len(decs) == 0 {
				t.Logf("ZDEC: after char, A=ci=%d Z=%d hl=$%04X 9A80+raw=$%02X raw=%q", emu.cpu.A, func() int {
					if emu.cpu.F&0x40 != 0 {
						return 1
					}
					return 0
				}(), emu.cpu.HL(), emu.mem.Read(uint16(0x9A80)+uint16(emu.mem.Read(emu.cpu.IX-1))), string(rune(emu.mem.Read(emu.cpu.IX-1))))
			}
		}
		if emu.cpu.PC == 0x81D0 { // cp $FF / jp z,gbump — A=ci, IX=next char pos
			if len(decs) < 3 {
				t.Logf("RAM@walk: CHARID+raw($%02X)=$%02X  CHARID[20]=$%02X CHARID[77]=$%02X sptrtab=$%04X", emu.mem.Read(emu.cpu.IX-1), emu.cpu.A, emu.mem.Read(0x9AA0), emu.mem.Read(0x9AF7), rdw(uint16(0x8C14)))
			}
			decs = append(decs, dec{uint16(emu.cpu.IX), uint16(emu.mem.Read(emu.cpu.IX - 1)), uint16(emu.cpu.A)})
		}
		if emu.cpu.PC == 0x8249 {
			writes = append(writes, wr{uint16(emu.cpu.A << 8), uint16(emu.cpu.DE()), emu.cpu.HL()})
		}
		emu.cpu.StepInstruction()
		steps++
	}
	for i, r := range regs[:min(30, len(regs))] {
		t.Logf("R%02d $%04X af=%04X bc=%04X de=%04X hl=%04X ix=%04X iy=%04X", i, r.pc, r.af, r.bc, r.de, r.hl, r.ix, r.iy)
	}
	for i, d := range decs[:min(24, len(decs))] {
		t.Logf("G%d ix=$%04X raw=%q ci=%d", i, d.ix-1, string(rune(d.raw)), d.ci)
	}
	t.Logf("walk executed %d writes", len(writes))
	for i, w := range writes[:min(24, len(writes))] {
		t.Logf("W%d val=%02X dest=$%04X sprptr=$%04X", i, w.a>>8, w.de, w.hl-1)
	}
	t.Logf("walk ran %d steps, PC=$%04X", steps, emu.cpu.PC)

	after := emu.mem.RAM8KPage(10)[:0x4000]
	// predicted ink from globals
	p := rdw(sym("p"))
	lvl := ram(sym("lvl"))
	pbyte := p >> 3
	q := pbyte >> lvl
	nb := uint16(1) << lvl
	cb0 := int(int8(q*nb - pbyte))
	hh := ram(sym("hh"))
	half := ram(sym("half"))
	lptrb := ram(sym("lptrb"))
	yoff := func(x uint16) byte { return ram(sym("YOFF") + x) }
	deftab := func(y uint16) uint16 {
		return uint16(emu.mem.Read(sym("DEFTAB")+y*2)) | uint16(emu.mem.Read(sym("DEFTAB")+y*2+1))<<8
	}
	virt := make([]byte, 0x4000)
	ink := func(a uint16, v byte) { virt[a-0x4000] |= v }
	for g := 0; g < 32; g++ {
		cbb := cb0 + g*int(nb)
		if cbb >= 32 {
			break
		}
		rawc := ram(sym("TEXT") + q + uint16(g))
		ci := ram(sym("CHARID") + uint16(rawc))
		if ci == 0xFF {
			continue
		}
		sptr := rdw(0x8C00 + uint16(lptrb) + uint16(ci)*2)
		x := uint16(cbb*8 + int(half))
		if x > 255 {
			x = 0
		}
		ytop := int(yoff(x)) - int(half)
		span := int(nb)
		if rem := 32 - cbb; span > rem {
			span = rem
		}
		for r := 0; r < int(hh); r++ {
			for k := 0; k < span; k++ {
				if sb := ram(sptr + uint16(r)*nb + uint16(k)); sb != 0 {
					ink(deftab(uint16(ytop+r))+uint16(cbb+k), sb)
				}
			}
		}
	}
	mism := [][4]int{}
	for o := 0; o < 0x4000; o++ {
		exp := virt[o]
		got := after[o]
		if exp != 0 && got&exp != exp {
			a := uint16(0x4000 + o)
			y := ((a-0x4000)>>8)&7 | (((a-0x4000)>>5)&7)<<3 | (((a-0x4000)>>11)&3)<<6
			x := (o & 31) * 8
			mism = append(mism, [4]int{int(a), int(y), x, int(exp)})
		}
	}
	t.Logf("post-walk ink mismatches: %d", len(mism))
	for i, m := range mism[:15] {
		t.Logf("MISM[%d] addr=$%04X y=%d xbyte=%d exp=%08b got=%08b", i, m[0], m[1], m[2], m[3], after[m[0]-0x4000])
	}
	// where did walk actually paint vs before?
	painted := 0
	diffAddr := []int{}
	for o := 0; o < 0x4000; o++ {
		if after[o] != before[o] {
			painted++
			if len(diffAddr) < 12 {
				a := 0x4000 + o
				y := ((o>>8)&7 | ((o>>5)&7)<<3 | ((o>>11)&3)<<6)
				diffAddr = append(diffAddr, a)
				t.Logf("walk wrote $%04X (y=%d x=%d) %08b -> %08b", a, y, (o&31)*8, before[o], after[o])
			}
		}
	}
	t.Logf("walk wrote %d bytes", painted)
}

// second half: render predicted ink vs actual screen as ASCII side by side.
func TestSinescrollProbeRender(t *testing.T) {
	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "EQU 0x") {
				if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
					syms[name] = uint16(v)
				}
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
	raw, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	var bdata []byte
	var baddr uint16
	off := 0
	var pend uint16
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
			baddr, bdata = pend, p[1:n-1]
		}
	}
	emu, _ := newEmulator(roms.ModelPlus2)
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bdata {
		emu.mem.Write(baddr+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000
	emu.cpu.IM = 1
	halt := sym("lhalt")
	for i := 0; i < 400; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.cpu.PC == halt+1 && emu.mem.Read(sym("lvl")) == 0 && i > 100 {
			t.Logf("parked f%d", i)
			break
		}
	}
	rdw := func(a uint16) uint16 { return uint16(emu.mem.Read(a)) | uint16(emu.mem.Read(a+1))<<8 }
	ram := func(a uint16) byte { return emu.mem.Read(a) }
	op := rdw(sym("op"))
	olvl := ram(sym("olvl"))
	hh := ram(sym("hh"))
	half := ram(sym("half"))
	lptrb := ram(sym("lptrb"))
	// predicted virtual screen
	virt := make([]byte, 0x4000)
	ink := func(a uint16, v byte) { virt[a-0x4000] |= v }
	pbyte := op >> 3
	q := pbyte >> olvl
	nb := uint16(1) << olvl
	cb0 := int(int8(q*nb - pbyte))
	// gspan/grem semantics: clip right edge
	for g := 0; g < 32; g++ {
		cbb := cb0 + g*int(nb)
		if cbb >= 32 {
			break
		}
		rawc := ram(sym("TEXT") + q + uint16(g))
		ci := ram(sym("CHARID") + uint16(rawc))
		if ci == 0xFF {
			continue
		}
		sptr := rdw(0x8C00 + uint16(lptrb) + uint16(ci)*2)
		x := uint16(cbb*8 + int(half))
		if x > 255 {
			x = 0
		}
		ytop := int(emu.mem.Read(sym("YOFF")+x)) - int(half)
		span := int(nb)
		if rem := 32 - cbb; span > rem {
			span, _ = rem, 0
		}
		for r := 0; r < int(hh); r++ {
			for k := 0; k < span; k++ {
				sb := ram(sptr + uint16(r)*nb + uint16(k))
				if sb != 0 {
					ink(uint16(uint16(emu.mem.Read(sym("DEFTAB")+uint16(ytop+r)*2))|uint16(emu.mem.Read(sym("DEFTAB")+uint16(ytop+r)*2+1))<<8)+uint16(cbb+k), sb)
				}
			}
		}
	}
	// side-by-side ASCII: columns 0..95 px
	sc := emu.mem.RAM8KPage(10)
	t.Logf("=== predicted (left) vs actual (right), px x0..95 ===")
	for y := 80; y < 160; y++ {
		var pa, aa strings.Builder
		for x := 0; x < 96; x++ {
			o := ((y&7)<<8 | (y&0x38)<<2 | (y&0xC0)<<5 | (x >> 3))
			pb := virt[o]
			ab := sc[o]
			pa.WriteByte(px(pb, x))
			aa.WriteByte(px(ab, x))
		}
		if strings.ContainsAny(pa.String()+aa.String(), "#") {
			t.Logf("y%d %-96s | %s", y, pa.String(), aa.String())
		}
	}
}

func px(b byte, x int) byte {
	if b&(0x80>>(x&7)) != 0 {
		return '#'
	}
	return '.'
}

// TestSinescrollGlyphTrace — park, force draw, step walk logging every
// screen write with (glyph cb, sprite row r, byte k) so we see exactly how
// sprite rows map to screen addresses.
func TestSinescrollGlyphTrace(t *testing.T) {
	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "EQU 0x") {
				if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
					syms[name] = uint16(v)
				}
			}
		}
	}
	sym := func(n string) uint16 { return syms[n] }
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	if err != nil {
		t.Fatal(err)
	}
	var pend, baddr uint16
	var bdata []byte
	off := 0
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
			baddr, bdata = pend, p[1:n-1]
			break
		}
	}
	if len(bdata) != 0x22F0 {
		t.Fatalf("bad block len %d", len(bdata))
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
	emu.cpu.PC = 0x8000
	emu.cpu.IM = 1
	for i := 0; i < 400; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.cpu.PC == sym("lhalt")+1 && emu.mem.Read(sym("lvl")) == 0 && i > 100 {
			break
		}
	}
	// force DRAW pass and run walk
	emu.mem.Write(sym("paint")+1, 0xFF)
	halt := sym("lhalt")
	sp := emu.cpu.SP - 2
	emu.mem.Write(sp, byte(halt))
	emu.mem.Write(sp+1, byte(halt>>8))
	emu.cpu.SP = sp
	emu.cpu.PC = sym("walk")
	emu.cpu.Halted = false
	t.Logf("bytes at paint: %02X %02X %02X ; halt=%04X", emu.mem.Read(sym("paint")), emu.mem.Read(sym("paint")+1), emu.mem.Read(sym("paint")+2), halt)
	ram := func(a uint16) byte { return emu.mem.Read(a) }
	rdw := func(a uint16) uint16 { return uint16(emu.mem.Read(a)) | uint16(emu.mem.Read(a+1))<<8 }
	type W struct {
		cb, r, k int
		byteVal  byte
		dest     uint16
		x, y     int
		raw      byte
		ci       byte
	}
	var ws []W
	var cbSeen []int
	gci := 0
	dec := func(a uint16) (int, int) {
		q := int(a) - 0x4000
		third := (q >> 11) & 3
		prow := (q >> 8) & 7
		crow := (q >> 5) & 7
		xcol := q & 31
		return third*64 + crow*8 + prow, xcol*8 + 7
	}
	// glyph tracker: at $81C0 (cp $20 jp nc,w_done) A=(cb)
	// sprite row counter: at $823E ld b,a region? use row loop marker: DEFTAB fetch PC $8230-ish...
	// simpler: log writes + cb; reconstruct r,k by ordering
	steps := 0
	lastCB := -1
	for emu.cpu.PC != halt+1 && steps < 200000 {
		if emu.cpu.PC == 0x81C0 { // cp $20 ; jp nc,w_done
			lastCB = int(emu.cpu.A)
			cbSeen = append(cbSeen, lastCB)
		}
		if emu.cpu.PC == 0x81D0 { // cp $FF
			gci = int(emu.cpu.A)
			if len(cbSeen) < 3 {
				t.Logf("FETCH ix=$%04X raw=%q hl=$%04X char=[$%02X] nxt=[$%02X] ci=%d", emu.cpu.IX, string(rune(emu.mem.Read(emu.cpu.IX-1))), emu.cpu.HL(), emu.mem.Read(emu.cpu.HL()), emu.mem.Read(emu.cpu.IX), emu.cpu.A)
			}
		}
		if emu.cpu.PC == 0x81CE { // add hl,de
			if len(cbSeen) < 3 {
				t.Logf("ADD81CC hl=$%04X e=$%02X a=$%02X sp=$%04X bytes=$81C8:%02X%02X%02X $81CB:%02X $81CC:%02X", emu.cpu.HL(), emu.cpu.E, emu.cpu.A, emu.cpu.SP, emu.mem.Read(0x81C8), emu.mem.Read(0x81C9), emu.mem.Read(0x81CA), emu.mem.Read(0x81CB), emu.mem.Read(0x81CE))
			}
		}
		if emu.cpu.PC == 0x8249 {
			de := emu.cpu.DE()
			y, x := dec(de)
			ws = append(ws, W{cb: lastCB, byteVal: emu.cpu.A, dest: de, x: x, y: y, raw: ram(emu.cpu.IX - 1), ci: byte(gci)})
		}
		emu.cpu.StepInstruction()
		steps++
	}
	t.Logf("walk steps=%d writes=%d glyphs=%d", steps, len(ws), len(cbSeen))
	for i, w := range ws[:min(60, len(ws))] {
		t.Logf("w%-3d cb=%-2d ci=%-3d raw=%q val=%02X dest=$%04X -> (x%d,y%d)", i, w.cb, w.ci, string(rune(w.raw)), w.byteVal, w.dest, w.x, w.y)
	}
	_ = rdw
}
