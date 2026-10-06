package main

// TestSinescrollStall — v6.2 ground-truth PC accounting per paint cycle.
// Accumulate per-PC executed count + T for the WHOLE cycle (eraseband entry
// to next). Dump top PCs so the "90k T cycle" is attributed to instructions,
// not folklore. Cycle = eraseband..eraseband (covers erase+walk+tail+park).
import (
	"fmt"
	"os"
	"sort"
	"testing"
)

func TestSinescrollStall(t *testing.T) {
	if _, err := os.Stat("/root/nerve-workspace/demos/sinescroll/sinescroll.tap"); err != nil {
		t.Skip("no tap")
	}
	syms := loadSinescrollSyms(t)
	emu := bootSinescroll(t, syms, "/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	type acc struct {
		n uint64
		t uint64
	}
	hist := map[uint16]*acc{}
	// ordered region starts -> summed T within the captured cycle
	regNames := []struct {
		name string
		lo   uint16
	}{
		{"park", syms["loop"]}, {"lhalt", syms["lhalt"]},
		{"proc", syms["proc"]}, {"pclean", syms["pclean"]},
		{"walk", syms["walk"]}, {"gloop", syms["gloop"]},
		{"g_fast8", syms["g_fast8"]}, {"u8f", syms["u8f"]},
		{"g_slow", syms["g_slow"]}, {"eraseband", syms["eraseband"]},
		{"bandsave", syms["bandsave"]}, {"isr", syms["isr"]},
		{"tail", 0x8C00},
	}
	regT := make([]uint64, len(regNames))
	inCycle := false
	prevPC := syms["eraseband"] // dt belongs to the instruction just executed
	// witness every $7FFD paging write + flip stage state at capture
	flipLog := []string{}
	emu.mem.SetPagingTracer(func(source string, val byte, applied, sb, sa bool) {
		if len(flipLog) < 60 {
			flipLog = append(flipLog, fmt.Sprintf("OUT %02X applied=%v src=%s @t=%d", val, applied, source, emu.cpu.Tstates()))
		}
	})
	eb := 0
	var lastT uint64
	var tE0 uint64
	lvlAt := byte(0xFF)
	hpAt := byte(0xFF)
	// census displayed-chip lit bytes at EVERY parked wake
	parks, maxL, minL := 0, 0, 1<<30
	var bbkSeen = map[byte]int{}
	for i := 0; i < 1200000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		if emu.cpu.PC == syms["lhalt"]+1 && parks < 3 {
			litOf := func(pg int) int {
				chip := emu.mem.RAM8KPage(pg)
				n := 0
				for _, b := range chip[:6144] {
					if b != 0 {
						n++
					}
				}
				return n
			}
			bbk := emu.mem.Read(syms["bbk"])
			w4000 := uint16(emu.mem.Read(0x4001))<<8 | uint16(emu.mem.Read(0x4000))
			wC000 := uint16(emu.mem.Read(0xC001))<<8 | uint16(emu.mem.Read(0xC000))
			fmt.Printf("PARK bbk=%02X pg04=%d pg05=%d pg10=%d pg14=%d win4000=%04X winC000=%04X\n",
				bbk, litOf(4), litOf(5), litOf(10), litOf(14), w4000, wC000)
		}
		if emu.cpu.PC == syms["lhalt"]+1 {
			parks++
			litOf := func(pg int) int {
				chip := emu.mem.RAM8KPage(pg)
				n := 0
				for _, b := range chip {
					if b != 0 {
						n++
					}
				}
				return n
			}
			lit := litOf(5) + litOf(4)
			if emu.mem.Read(syms["bbk"])&0x08 != 0 {
				lit = litOf(14) + litOf(13)
			}
			bbkSeen[emu.mem.Read(syms["bbk"])]++
			if lit > maxL {
				maxL = lit
			}
			if lit < minL {
				minL = lit
			}
		}
		pc := emu.cpu.PC
		t := emu.cpu.Tstates()
		dt := t - lastT
		lastT = t
		if inCycle {
			ri := len(regNames) - 1
			for j := len(regNames) - 1; j >= 0; j-- {
				if prevPC >= regNames[j].lo {
					ri = j
					break
				}
			}
			regT[ri] += dt
		}
		prevPC = pc
		a := hist[pc]
		if a == nil {
			a = &acc{}
			hist[pc] = a
		}
		a.n++
		a.t += dt
		if pc == syms["eraseband"] {
			eb++
			if eb == 26 {
				inCycle = true
				lvlAt = emu.mem.Read(syms["lvl"])
				hpAt = emu.mem.Read(syms["hp"])
				tE0 = t
				hist = map[uint16]*acc{} // attribute THIS cycle only
			} else if eb == 27 && tE0 != 0 {
				inCycle = false
				fmt.Printf("CYC lvl=%d hp=%d ot[%d..%d total=%dT\n", lvlAt, hpAt,
					emu.mem.Read(syms["otmin"]), emu.mem.Read(syms["otmax"]), t-tE0)
				fmt.Printf("bbk=%02X flipdirty=%d pgWrites=%d\n",
					emu.mem.Read(syms["bbk"]), emu.mem.Read(syms["flipdirty"]), len(flipLog))
				fmt.Printf("PARKS n=%d maxLit=%d minLit=%d bbk=%v\n", parks, maxL, minL, bbkSeen)
				// lit census of every physical 8K page 4..15 at cycle end
				for pg := 4; pg < 16; pg++ {
					chip := emu.mem.RAM8KPage(pg)
					n := 0
					for _, b := range chip {
						if b != 0 {
							n++
						}
					}
					if n > 0 {
						fmt.Printf("  pg%02d lit=%d/%d\n", pg, n, len(chip))
					}
				}
				for j, s := range flipLog {
					if j >= 12 {
						break
					}
					fmt.Println("  FLIP " + s)
				}
				var rs uint64
				for j, rn := range regNames {
					if regT[j] > 400 {
						fmt.Printf("  %-10s T=%d\n", rn.name, regT[j])
					}
					rs += regT[j]
				}
				fmt.Printf("  %-10s T=%d\n", "regs-sum", rs)
				type kv struct {
					k uint16
					v *acc
				}
				var arr []kv
				for k, v := range hist {
					arr = append(arr, kv{k, v})
				}
				sort.Slice(arr, func(x, y int) bool { return arr[x].v.t > arr[y].v.t })
				var sum uint64
				for j := 0; j < len(arr) && j < 24; j++ {
					fmt.Printf("PC $%04X n=%d T=%d\n", arr[j].k, arr[j].v.n, arr[j].v.t)
					sum += arr[j].v.t
				}
				fmt.Printf("top24 sum=%dT\n", sum)
				return
			}
		}
	}
}
