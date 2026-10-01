package main

// Throwaway: authoritative per-COLUMN cost inside the scroller.
// Hooks all 8 SBAND loop heads (ld a,(iy+1) = FD 7E 01, $8470..$85D5
// $33 apart) + E3@81D1 (border before call sscroll) + INT@8989.
// Delta between consecutive hits at the SAME site = exact T cost of one
// band byte at the current sval. Also captures sval AT E3 (before the
// call, i.e. the value THIS frame's paint will use — sval is recomputed
// inside sscroll, so reading it at E0 sees the same frame's value only
// by luck; keying must happen before the call).
// Delete when the scroller budget is locked.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowScrollerCost(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	tapPath := "/root/nerve-workspace/demos/snow/snow.tap"
	if p := os.Getenv("SNOW_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatal(err)
	}
	var code []byte
	off := 0
	for off+2 <= len(raw) {
		n := int(raw[off]) | int(raw[off+1])<<8
		off += 2
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0xFF && n > 5000 {
			code = p[1 : n-1]
		}
	}
	if code == nil {
		t.Fatal("no big CODE block")
	}
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(uint16(0x8000+i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00)
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	emu.mem.Write(0x8369, 0xAF) // kfull stub: no ghost-key quits
	emu.mem.Write(0x836A, 0xC9)

	sbSites := []uint16{0x8470, 0x84A3, 0x84D6, 0x8509, 0x853C, 0x856F, 0x85A2, 0x85D5}
	svalA := uint16(0x8CA0)

	var lastAt [8]int64      // last Tstates at each .sb site
	var lastS [8]uint8        // sval in force at that hit
	var tE3 int64            = -1 // Tstates at E3 (this frame's paint start)
	var rowArmed [8]bool
	var sPaint uint8          // sval of the paint run being measured (row-0 anchor)
	var colDeltas []struct { // per-column costs this captured window
		s    uint8
		t    int64
		site int
	}
	var frameRecs []struct { // per scroller-execution spans
		s        uint8
		span, by uint64
	}
	var e0, e3cnt, ints int
	var tE0Prev int64 = -1

	emu.cpu.AddPreFetchHook("cost", func(pc uint16) {
		t := int64(emu.cpu.Tstates())
		for i, a := range sbSites {
			if pc != a {
				continue
			}
			if !rowArmed[i] {
				// First column of this row: authoritative sval anchor.
				rowArmed[i] = true
				s := uint8(emu.mem.Read(svalA))
				lastAt[i] = t
				lastS[i] = s
				if i == 0 {
					sPaint = s // the phase THIS paint run uses
				}
				return
			}
			if lastAt[i] <= t && lastS[i] == sPaint && tE3 >= 0 {
				colDeltas = append(colDeltas, struct {
					s    uint8
					t    int64
					site int
				}{lastS[i], t - lastAt[i], i})
			}
			lastAt[i] = t
			return
		}
		switch pc {
		case 0x81D1: // E3: paint start; sval finalized INSIDE sscroll, so
			// the phase key is anchored at row-0's first column below.
			e3cnt++
			tE3 = t
			// re-arm row anchors: first column of each row re-captures sval
			for i := range lastAt {
				lastAt[i] = 0
				rowArmed[i] = false
			}
		case 0x8114: // E0: paint finished, back at loop head
			e0++
			if tE3 >= 0 && t >= tE3 { // spans crossing T-wrap are dropped
				n := 0
				for _, c := range colDeltas {
					if c.s == sPaint {
						n++
					}
				}
				frameRecs = append(frameRecs, struct {
					s        uint8
					span, by uint64
				}{sPaint, uint64(t - tE3), uint64(n)})
				tE3 = -1
			}
			tE0Prev = t
		case 0x8989:
			ints++
		}
	})
	nf2 := 400
	if v := os.Getenv("SNOW_COST_FRAMES"); v != "" {
		fmt.Sscanf(v, "%d", &nf2)
	}
	for f := 0; f < nf2; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	_ = tE0Prev

	fmt.Printf("COUNTS e0=%d e3=%d int=%d cols=%d frames=%d\n",
		e0, e3cnt, ints, len(colDeltas), len(frameRecs))
	if len(frameRecs) == 0 {
		t.Fatal("no scroller frames captured")
	}

	// per-phase: scroller span and per-column cost
	type agg struct {
		n     int
		span  uint64
		cost  uint64
		cmax  int64
		cmin  int64
	}
	byS := map[uint8]*agg{}
	sites := map[[2]int]*agg{}
	for _, c := range colDeltas {
		a := byS[c.s]
		if a == nil {
			a = &agg{cmin: 1 << 62}
			byS[c.s] = a
		}
		a.n++
		a.cost += uint64(c.t)
		if c.t > a.cmax {
			a.cmax = c.t
		}
		if c.t < a.cmin {
			a.cmin = c.t
		}
		k := [2]int{int(c.s), c.site}
		sa := sites[k]
		if sa == nil {
			sa = &agg{cmin: 1 << 62}
			sites[k] = sa
		}
		sa.n++
		sa.cost += uint64(c.t)
	}
	var sumSpan uint64
	var maxSpan uint64
	var worstU uint8
	spanBy := map[uint8][]uint64{}
	for _, r := range frameRecs {
		sumSpan += r.span
		if r.span > maxSpan {
			maxSpan, worstU = r.span, r.s
		}
		spanBy[r.s] = append(spanBy[r.s], r.span)
	}
	fmt.Printf("SCROLLER span: avg=%d max=%d (worst s=%d)\n",
		sumSpan/uint64(len(frameRecs)), maxSpan, worstU)
	ks := []int{}
	for k := range byS {
		ks = append(ks, int(k))
	}
	sort.Ints(ks)
	fmt.Printf("  phase  frames  spanAvg   colT avg/min/max\n")
	for _, k := range ks {
		a := byS[uint8(k)]
		sb := spanBy[uint8(k)]
		var sav uint64
		for _, v := range sb {
			sav += v
		}
		if len(sb) > 0 {
			sav /= uint64(len(sb))
		}
		fmt.Printf("  s=%d    n=%3d   %7d   %6d/%d/%d\n",
			k, len(sb), sav, a.cost/uint64(a.n), a.cmin, a.cmax)
	}
	if os.Getenv("SNOW_COST_ROWS") != "" {
		rk := []int{}
		for k := range sites {
			rk = append(rk, k[0]*100+k[1])
		}
		sort.Ints(rk)
		for _, v := range rk {
			k := [2]int{v / 100, v % 100}
			a := sites[k]
			fmt.Printf("  s=%d row%d n=%3d colAvg=%d\n", k[0], k[1], a.n, a.cost/uint64(a.n))
		}
	}
}
