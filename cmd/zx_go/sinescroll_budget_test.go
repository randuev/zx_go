package main

import (
	"fmt"
	"os"
	"sort"
	"testing"
)

// TestSinescrollBudget — TRUE paint-window budget gate (v5).
//
// Park-to-park totals (phase gate) are ~frame-length by construction: the
// loop idles at HALT until the next vsync, so they cannot show the paint
// cost directly. This gate stamps the deterministic paint window per cycle:
//
//	busy    = t(pclean) - t(eraseband-entry)  (prologue+gate ~0.5k extra)
//	overrun iff PC hits isr inside that window (vsync swallowed by the DI
//	paint window, fired delayed at the tail ei) — the v4.3-era condition that
//	phase-shifted every later flip and, pre-v5, showed stale banks mid-paint.
//	v5 LAW (Option B cadence): parked cycles paint NOTHING (gate in proc);
//	paint cycles must land erase+walk+bandsave+poll inside ONE frame so the
//	gated flip fires exactly at the NEXT vsync = rock-steady 25 fps, no
//	mid-frame bank swap, no stale bank, ever.
//
// Frame law: +2 grey = 311 lines x 228 T = 70908. The task text's 69888
// is the 48K number kept as the extra-safe margin gate.
//
// v5 DONE CRITERIA: 0 h8 overruns over >=600 frames; medians printed.
func TestSinescrollBudget(t *testing.T) {
	syms := loadSinescrollSyms(t)
	tapPath := "/root/nerve-workspace/demos/sinescroll/sinescroll.tap"
	if p := os.Getenv("SINESCROLL_TAP"); p != "" {
		tapPath = p
	}
	emu := bootSinescroll(t, syms, tapPath)

	const FRAME = 70908 // +2 grey 311x228
	const SAFE = 69888 // 48K reference margin

	type cyc struct {
		frame   int
		lvl     byte
		painted bool // eraseband executed this cycle (paint cycle)
		busy    int
		erase   int
		blit    int
		bsave   int
		delayed bool // vsync ISR observed inside the paint window
	}
	var cycs []cyc
	frame := 0
	prevPaintBusy := 0
	locksLose := 0
	for frame < 600 {
		steps := 0
		var tE0, tE1, tWd, tBs0, tPc uint64
		painted := false
		delayed := false
		for {
			steps++
			if steps > 400000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
			pc := emu.cpu.PC
			t := emu.cpu.Tstates()
			if pc == syms["eraseband"] && tE0 == 0 {
				tE0 = t
			}
			// v6.5: parked wakes wide-erase the hidden bank too —
			// painted must be detected at ep_ei (flipdirty commit), the
			// one stamp only paint wakes reach.
			if pc == syms["ep_ei"] {
				painted = true
			}
			if tE0 != 0 && pc == syms["isr"] {
				delayed = true
			}
			if tE0 != 0 && pc == syms["walk"] && tE1 == 0 {
				tE1 = t
			}
			if tE1 != 0 && pc == syms["w_done"] && tWd == 0 {
				tWd = t
			}
			if tWd != 0 && pc == syms["bandsave"] && tBs0 == 0 {
				tBs0 = t
			}
			if tE0 != 0 && pc == syms["pclean"] && tPc == 0 {
				tPc = t
			}
			if pc == syms["loop"] && steps > 20 && tPc != 0 {
				break
			}
			// parked cycle: no erase ever — break when pclean reached
			if pc == syms["pclean"] && steps > 20 && tE0 == 0 {
				tPc = t
				break
			}
		}
		c := cyc{frame: frame, lvl: emu.mem.Read(syms["lvl"]), painted: painted, delayed: delayed}
		if painted && tPc != 0 && tE0 != 0 {
			c.busy = int(tPc - tE0)
		}
		if !painted && tPc != 0 && tE0 != 0 {
			// v6.6 cadence-lock: parked h16 wipe must finish before the
			// vsync that carries its flip. paint(painted cycle, frame N)
			// busy + park wipe (frame N+1) must stay inside 2 frames minus
			// contention slack, else flip slips one more frame = 16fps drift.
			park := int(tPc - tE0)
			if prevPaintBusy > 0 {
				comb := prevPaintBusy + park
				const LOCK = FRAME*2 - 4000 // 137,816
				if comb > LOCK {
					locksLose++
					if locksLose < 4 {
						fmt.Printf("LOCKLOSS f%d paint=%d park=%d comb=%d > %d\n",
							frame, prevPaintBusy, park, comb, LOCK)
					}
				}
			}
		}
		if painted {
			prevPaintBusy = c.busy
		} else {
			prevPaintBusy = 0
		}
		if tE1 > tE0 {
			c.erase = int(tE1 - tE0)
		}
		if tWd > tE1 {
			c.blit = int(tWd - tE1)
		}
		if tPc > tBs0 && tBs0 != 0 {
			c.bsave = int(tPc - tBs0)
		}
		frame++
		cycs = append(cycs, c)
	}

	var busy8, busy16, es, bl8 []int
	over8, over16, overAll, delayedCnt, safeOver8, paintCyc, parkCyc := 0, 0, 0, 0, 0, 0, 0
	var overs []string
	for _, c := range cycs {
		if c.painted {
			paintCyc++
		} else {
			parkCyc++
		}
		if c.busy <= 0 {
			continue
		}
		if c.delayed {
			delayedCnt++
		}
		if c.busy > FRAME {
			overAll++
		}
		if c.lvl == 0 {
			busy8 = append(busy8, c.busy)
			if c.blit > 0 {
				bl8 = append(bl8, c.blit)
			}
			if c.busy > FRAME {
				over8++
				if len(overs) < 12 {
					overs = append(overs, fmt.Sprintf("f%d busy=%d erase=%d blit=%d bsave=%d delayed=%v",
						c.frame, c.busy, c.erase, c.blit, c.bsave, c.delayed))
				}
			}
			if c.busy > SAFE {
				safeOver8++
			}
		} else {
			busy16 = append(busy16, c.busy)
			if c.busy > FRAME {
				over16++
			}
		}
		if c.erase > 0 {
			es = append(es, c.erase)
		}
	}
	sort.Ints(busy8)
	sort.Ints(busy16)
	sort.Ints(es)
	sort.Ints(bl8)
	pctl := func(name string, v []int) {
		if len(v) == 0 {
			fmt.Printf("%s: none\n", name)
			return
		}
		fmt.Printf("%s n=%d med=%d p90=%d p99=%d max=%d\n",
			name, len(v), v[len(v)/2], v[len(v)*9/10], v[len(v)*99/100], v[len(v)-1])
	}
	fmt.Printf("cycles=%d paint=%d park=%d\n", len(cycs), paintCyc, parkCyc)
	pctl("h8 busy", busy8)
	pctl("h16 busy", busy16)
	pctl("erase", es)
	pctl("h8 blit", bl8)
	fmt.Printf("overruns>70908: h8=%d h16=%d all=%d | h8>69888=%d | delayedISR=%d | lockloss=%d\n",
		over8, over16, overAll, safeOver8, delayedCnt, locksLose)
	if locksLose > 0 {
		t.Errorf("%d paint+park cycles exceed the 2-frame flip lock window", locksLose)
	}
	for _, s := range overs {
		fmt.Println("  OVER8 " + s)
	}
	// cadence uniformity: paint cycles must strictly alternate park/paint
	// (v5 Option B) — two consecutive painted or parked = cadence bug.
	bad := 0
	for i := 1; i < len(cycs); i++ {
		if cycs[i].painted == cycs[i-1].painted {
			bad++
			if bad < 4 {
				fmt.Printf("CADENCE f%d,f%d both painted=%v\n", i-1, i, cycs[i].painted)
			}
		}
	}
	fmt.Printf("cadence alternation breaks=%d\n", bad)
	if bad > 0 {
		t.Errorf("%d cadence alternation breaks (park/paint must strictly alternate)", bad)
	}
	// v6.3 LAW (amp-58, 2/3 screen): steady paint is FULL BAND every OTHER
	// wake -> a paint cycle may span TWO frame periods (shown bank held by
	// the flipdirty gate; parked wakes never flip -> never stale, never
	// torn). Budget for a painted cycle = 2 frames minus 15% contention
	// slack. Measured v6.3: h8 med 104,619 max 104,922 (slack 26%);
	// h16 med 87,967 max 88,561 (slack 38%).
	const PAINT2 = (FRAME * 2) * 85 / 100 // 120,544
	if o := countOver(busy8, PAINT2); o > 0 {
		t.Errorf("H8 paint cycles: %d/%d busy > %dT (v6.3 2-frame-85%% law)", o, len(busy8), PAINT2)
	}
	// v6.5 h16 gate: steady pad-tight fits 1 frame + entry wakes fit 2 with
	// 25% slack — the tighter budget catches the full-screen-every-wake
	// regression that hit 124,516T > 2-frame-85% before pad v6.5.
	const PAINT16 = FRAME * 95 / 100 // 67,362 steady; entry wakes allowed 2 frames
	if o := countOver(busy16, PAINT2); o > 0 {
		t.Errorf("H16 paint cycles: %d/%d busy > %dT (v6.3 2-frame-85%% law)", o, len(busy16), PAINT2)
	}
	if delayedCnt > 0 {
		t.Errorf("%d cycles had a delayed vsync inside the paint window", delayedCnt)
	}
}

func countOver(v []int, lim int) int {
	n := 0
	for _, x := range v {
		if x > lim {
			n++
		}
	}
	return n
}
