package main

// Drain-cycle oracle (2026-10-01). The per-COLUMN scroller breakdown was
// unmeasurable — the main loop parks at HALT and a halted Z80 re-issues the
// M1 fetch at the SAME PC every 4T, so pre-fetch hooks anchored at the loop
// head were re-armed every halt tick (window reset forever -> zero frames).
// Instead: st_drained marks the end of a drain (~f1744, first frame after
// snow death). Arm the hook there and measure loop-cycle lengths over the
// frames that follow — by construction these include every revive/respawn
// paint during the next storm: the seamless-loop guarantee as a number.
// Delete when the scroller budget is locked.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// readSym parses sjasmplus --sym output ("name: EQU 0x0000NNNN").
// Strip ; comments, the EQU keyword, and $/0x prefixes — ParseUint with
// an explicit base REJECTS "0x..." (biting every lookup once silently).
func readSym(path, name string) (uint16, bool) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for _, ln := range strings.Split(string(bs), "\n") {
		n, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok || n != name {
			continue
		}
		t2 := strings.TrimSpace(rest)
		if j := strings.IndexByte(t2, ';'); j >= 0 {
			t2 = t2[:j]
		}
		if k := strings.Index(t2, "EQU"); k >= 0 {
			t2 = t2[k+3:]
		}
		t2 = strings.TrimSpace(t2)
		t2 = strings.TrimPrefix(t2, "$")
		t2 = strings.TrimPrefix(t2, "0x")
		if v, err := strconv.ParseUint(t2, 16, 16); err == nil {
			return uint16(v), true
		}
	}
	return 0, false
}

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

	base := uint16(0x8000)
	// loop head + drain marker via fresh syms; E3 via byte scan (stale hex
	// burned probes twice).
	loopA, ok := readSym("/root/nerve-workspace/demos/snow/snow.sym", "loop")
	if !ok || loopA == 0 {
		t.Fatal("loop sym missing")
	}
	drainedA, ok := readSym("/root/nerve-workspace/demos/snow/snow.sym", "st_drained")
	if !ok || drainedA == 0 {
		t.Fatal("st_drained sym missing")
	}
	e3A := uint16(0)
	for i := 0; i+7 < len(code); i++ {
		if code[i] == 0x3E && code[i+1] == 0xE3 && code[i+2] == 0xD3 && code[i+3] == 0xFE && code[i+4] == 0xCD {
			e3A = base + uint16(i)
			break
		}
	}
	if e3A == 0 {
		t.Fatal("E3 site not found")
	}
	t.Logf("anchors: loop=%04X st_drained=%04X e3=%04X", loopA, drainedA, e3A)

	var cyc []int64
	var tLoop int64 = -1
	var tE3 int64 = -1
	var cycWorst int64
	var spans []int64
	var spanWorst int64
	var armedAt, painted, frames int
	hooked := false
	emu.cpu.AddPreFetchHook("cost", func(pc uint16) {
		if !hooked {
			if pc == drainedA {
				hooked = true
				armedAt = frames
			}
			return
		}
		switch pc {
		case loopA:
			frames++
			t := int64(emu.cpu.Tstates())
			if tLoop >= 0 {
				c := t - tLoop
				if c > 0 { // negative = T rebased at frame boundary; drop
					cyc = append(cyc, c)
					if c > cycWorst {
						cycWorst = c
					}
				}
			}
			tLoop = t
			if tE3 >= 0 {
				sp := t - tE3
				if sp > 0 {
					spans = append(spans, sp)
					if sp > spanWorst {
						spanWorst = sp
					}
				}
				tE3 = -1
			}
		case e3A:
			painted++
			tE3 = int64(emu.cpu.Tstates())
		}
	})
	// Count pre-arm frames too (so armedAt is meaningful).
	var preFrames int
	emu.cpu.AddPreFetchHook("cost-pre", func(pc uint16) {
		if !hooked && pc == loopA {
			preFrames++
		}
	})

	nf2 := 2500
	if v := os.Getenv("SNOW_COST_FRAMES"); v != "" {
		fmt.Sscanf(v, "%d", &nf2)
	}
	for f := 0; f < nf2; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}

	fmt.Printf("armed at loop-head frame %d (pre-arm frames=%d)\n", armedAt, preFrames)
	if len(cyc) == 0 {
		t.Fatal("no post-drain loop cycles captured")
	}
	var sum int64
	for _, v := range cyc {
		sum += v
	}
	fmt.Printf("POST-DRAIN LOOP CYCLE (revive included): n=%d avg=%d worst=%d (%.1f%% of 69888)\n",
		len(cyc), sum/int64(len(cyc)), cycWorst, float64(cycWorst)/69888.0*100.0)
	if len(spans) > 0 {
		var ss int64
		for _, v := range spans {
			ss += v
		}
		fmt.Printf("SCROLLER span (E3->E0): n=%d avg=%d worst=%d\n",
			len(spans), ss/int64(len(spans)), spanWorst)
	}
	fmt.Printf("E3 paint executions captured: %d\n", painted)

	var tiny int
	for _, v := range cyc {
		if v < 16 {
			tiny++
		}
	}
	if tiny > 0 {
		t.Errorf("anchor polluted: %d cycles <16T (halt M1 refetch?)", tiny)
	}
	// Gate 55k -> 60k (2026-10-01): font x2 vertical stretch adds ~5.5k T
	// of LDIR row-mirrors to every scroller pass. Budget frame = 69,888 T;
	// 60k keeps a ~10k vsync-slack margin (E0->E3 re-arm headroom).
	if cycWorst > 60000 {
		t.Errorf("post-drain cycle worst %dT exceeds 60k budget", cycWorst)
	}
}
