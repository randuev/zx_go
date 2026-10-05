package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollBankHistory — backward-motion forensic on the SHIPPED tap:
// at every loop-top, record (frame, bbk, p, lvl, hash+inkrows of the bank
// about to be SHOWN at the next flip). MotionBug proof: if the ISR flips
// unconditionally but the dirty gate skips paints, the same bank contents
// reappear after a NEWER bank was shown => viewer sees text go BACK.
func TestSinescrollBankHistory(t *testing.T) {
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

	// Spy bandsave completion: bank stamped with (p,lvl) at commit time.
	// We snapshot the just-painted (hidden) bank content hash at loop-top,
	// tagged with the p that produced it, then watch the SHOW sequence:
	// the bank shown at loop-top N is the one painted at loop-top N-1.
	loop := syms["loop"]
	type rec struct {
		frame int
		bbk   byte
		p     uint16
		lvl   byte
		h     uint64
		ink   int
	}
	var recs []rec
	frames := 0
	haltPC := syms["lhalt"]
	for frames < 600 {
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
			_ = haltPC
		}
		frames++
		bbk := emu.mem.Read(syms["bbk"])
		p := uint16(emu.mem.Read(syms["p"])) | uint16(emu.mem.Read(syms["p"]+1))<<8
		lvl := emu.mem.Read(syms["lvl"])
		// DISPLAYED bank right now (what the CRT sees mid-frame):
		shownPage := 10
		if bbk&8 != 0 {
			shownPage = 14
		}
		g := emu.mem.RAM8KPage(shownPage)[:6144]
		hf := fnv.New64a()
		ink := 0
		var rows [192]int
		for y := 0; y < 192; y++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			for xb := 0; xb < 32; xb++ {
				v := g[o+xb]
				hf.Write([]byte{v})
				for b := 0; b < 8; b++ {
					if v&(1<<b) != 0 {
						ink++
						rows[y]++
					}
				}
			}
		}
		recs = append(recs, rec{frames, bbk, p, lvl, hf.Sum64(), ink})
	}
	// Analysis: paint cycle k produced record r_k (bank painted at p_k).
	// Show order = alternation: bank of cycle k shown at vsync k+1.
	// Backward event: consecutive SHOWN contents (cycle k then k+2, same bank)
	// where the newly shown bank is OLDER content than what was displayed in
	// between. Equivalent, simpler: hash of a bank unchanged across cycles
	// while the OTHER bank updated => the shown alternation goes stale->fresh.
	staleEvents := 0
	lvlHist := map[byte]int{}
	for _, r := range recs {
		lvlHist[r.lvl]++
	}
	// Detect "gate-skipped" cycles: hash identical to this bank's state at
	// cycle k-2 (bank alternates, so k vs k-2 is same bank).
	// We label skip if hidden hash same as hidden hash 2 cycles ago AND p
	// has advanced since (p advances even when paint skipped? NO: gate skips
	// paint AFTER p commit — p advances every frame regardless!).
	// DISPLAYED-sequence hazard: the shown image goes X -> Y -> X (X != Y):
	// that middle-to-last transition is text visibly going BACK. The p
	// printed beside each frame quantifies how far.
	skipsByLvl := map[byte]int{}
	hazardFrames := map[string]int{}
	for i := 2; i < len(recs); i++ {
		if recs[i].h == recs[i-2].h && recs[i-1].h != recs[i].h {
			skipsByLvl[recs[i].lvl]++
			staleEvents++
			if len(hazardFrames) < 12 {
				hazardFrames[fmt.Sprintf("f%d(p%d)", recs[i].frame, recs[i].p)] = int(recs[i-1].p) - int(recs[i].p)
			}
		}
	}
	fmt.Printf("hazardFrames=%v\n", hazardFrames)
	fmt.Printf("frames=%d X-Y-X-backward=%d byLvl=%v lvlHist=%v\n",
		len(recs), staleEvents, skipsByLvl, lvlHist)
	// Printed timeline samples: every recorded cycle first 200
	for i, r := range recs {
		if i < 200 {
			fmt.Printf("c%03d f%d bbk=%02X p=%3d lvl=%d ink=%4d h=%016X\n",
				i, r.frame, r.bbk, r.p, r.lvl, r.ink, r.h)
		}
	}
	// v4.3 LAW: no stale bank may ever be painted into a flip position —
	// backward motion is structurally impossible (always-paint + monotone p).
	if staleEvents != 0 {
		t.Errorf("STALE BANK RE-SHOW: %d cycles repainted identical hidden bank — backward risk (byLvl=%v)", staleEvents, skipsByLvl)
	}
}
