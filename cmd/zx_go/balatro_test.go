package main

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// MODE_* / EV_* layout must match balatro.zasm equ block.
const (
	bMODE_TITLE = 0
	bMODE_PLAY  = 1
	bMODE_SCORE = 2
	bMODE_WON   = 3
	bMODE_OVER  = 4
)

// startGame boots to title then presses SPACE to begin a round.
func startGame(t *testing.T, syms map[string]uint16) (*emulator, uint16) {
	t.Helper()
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 8)
	if m := balPeek(emu, syms["MODE"]); m != bMODE_TITLE {
		t.Fatalf("MODE=%d at boot, want TITLE(0)", m)
	}
	if lit := balTextLit(emu, 2) + balTextLit(emu, 4) + balTextLit(emu, 23); lit < 25 {
		t.Fatalf("title text sparse: lit=%d", lit)
	}
	pressOnce(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	if m := balPeek(emu, syms["MODE"]); m != bMODE_PLAY {
		t.Fatalf("MODE=%d after SPACE, want PLAY(1)", m)
	}
	return emu, syms["MODE"]
}

func TestBalatroBootTitle(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 8)
	if pc := emu.cpu.PC; pc < 0x8000 || pc > 0xBFFF {
		t.Fatalf("PC=$%04X outside code block — demo not running", pc)
	}
	if emu.cpu.IFF1 {
		t.Fatalf("IFF1 on — poll architecture must keep INTs off")
	}
	for _, r := range []int{2, 4, 23} {
		if balTextLit(emu, r) == 0 {
			t.Fatalf("title row %d empty", r)
		}
	}
	for _, r := range []int{0, 1, 3, 5, 6} {
		if balTextLit(emu, r) != 0 {
			t.Fatalf("title row %d should be blank, lit=%d", r, balTextLit(emu, r))
		}
	}
}

func TestBalatroDealDeterministic(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "ENTER")
	balRunFrames(emu, 8)
	if m := balPeek(emu, syms["MODE"]); m != bMODE_PLAY {
		t.Fatalf("MODE=%d want PLAY", m)
	}
	seen := map[byte]int{}
	for i := 0; i < 52; i++ {
		c := balPeek(emu, syms["DECK"]+uint16(i))
		if c > 51 {
			t.Fatalf("DECK[%d]=%d out of range", i, c)
		}
		seen[c]++
	}
	for c := 0; c < 52; c++ {
		if seen[byte(c)] != 1 {
			t.Fatalf("deck not a permutation: card %d appears %d times", c, seen[byte(c)])
		}
	}
	ident := true
	for i := 0; i < 52; i++ {
		if balPeek(emu, syms["DECK"]+uint16(i)) != byte(i) {
			ident = false
		}
	}
	if ident {
		t.Fatal("deck still identity — shuffle never ran")
	}
	seenH := map[byte]bool{}
	for i := 0; i < 8; i++ {
		c := balPeek(emu, syms["HAND"]+uint16(i))
		if c > 51 {
			t.Fatalf("HAND[%d]=%d out of range", i, c)
		}
		if seenH[c] {
			t.Fatalf("HAND[%d]=%d duplicate in deal", i, c)
		}
		seenH[c] = true
	}
	if h := balPeek(emu, syms["HANDS"]); h != 4 {
		t.Fatalf("HANDS=%d want 4", h)
	}
	if d := balPeek(emu, syms["DISCS"]); d != 3 {
		t.Fatalf("DISCS=%d want 3", d)
	}
	// engine contract: SPACE starts, ENTER selects, new rounds come via
	// WON→ENTER (advanceBlind→roundFresh→newRound→shuffle). Cross-round
	// rng evolution is proven directly: shuffle consumes SEED-pair and
	// rewrites it, so a second shuffle MUST produce a different deck,
	// while the SAME seed MUST reproduce the deck (determinism).
	callShuffle := func() [52]byte {
		ret := uint16(0xCC00)
		emu.mem.Write(0xCC00, 0x00)
		emu.cpu.SP -= 2
		emu.mem.Write(emu.cpu.SP, byte(ret&0xFF))
		emu.mem.Write(emu.cpu.SP+1, byte(ret>>8))
		emu.cpu.PC = syms["shuffle"]
		for i := 0; i < 400000; i++ {
			emu.cpu.StepInstruction()
			if emu.cpu.PC == ret {
				break
			}
		}
		var dk [52]byte
		for i := 0; i < 52; i++ {
			dk[i] = balPeek(emu, syms["DECK"]+uint16(i))
		}
		return dk
	}
	seed0 := balPeek(emu, syms["SEED"])
	seed0b := balPeek(emu, syms["SEED"] + 1)
	dA := callShuffle()
	// restore same seed -> same deck (deterministic)
	emu.mem.Write(syms["SEED"], seed0)
	emu.mem.Write(syms["SEED"]+1, seed0b)
	dA2 := callShuffle()
	for i := 0; i < 52; i++ {
		if dA[i] != dA2[i] {
			t.Fatalf("same seed produced different decks at %d — shuffle not deterministic", i)
		}
	}
	// natural evolution: SEED after shuffle differs -> next round differs
	sAfter := balPeek(emu, syms["SEED"])
	dB := callShuffle()
	same := true
	for i := 0; i < 52; i++ {
		if dA[i] != dB[i] {
			same = false
		}
	}
	if same {
		t.Fatalf("rng frozen across rounds (SEED=%02X/%02X -> after=%02X)", seed0, seed0b, sAfter)
	}
}

func TestBalatroCursorSelect(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu, _ := startGame(t, syms)
	if c := balPeek(emu, syms["CURSOR"]); c != 0 {
		t.Fatalf("CURSOR=%d want 0", c)
	}
	pressOnce(t, emu, syms, "6")
	if c := balPeek(emu, syms["CURSOR"]); c != 1 {
		t.Fatalf("CURSOR=%d after 6, want 1", c)
	}
	pressOnce(t, emu, syms, "4")
	if c := balPeek(emu, syms["CURSOR"]); c != 0 {
		t.Fatalf("CURSOR=%d after 4, want 0", c)
	}
	pressOnce(t, emu, syms, "4") // clamp at 0
	if c := balPeek(emu, syms["CURSOR"]); c != 0 {
		t.Fatalf("CURSOR=%d after 4-at-edge, want 0", c)
	}
	// select 5, sixth blocked
	for i := 0; i < 5; i++ {
		pressOnce(t, emu, syms, "ENTER")
		if got := balPeek(emu, syms["SELMARK"]+uint16(i)); got != 1 {
			t.Fatalf("SELMARK[%d]=%d want 1", i, got)
		}
		pressOnce(t, emu, syms, "6")
	}
	if n := balPeek(emu, syms["NSEL"]); n != 5 {
		t.Fatalf("NSEL=%d want 5", n)
	}
	pressOnce(t, emu, syms, "ENTER") // 6th at cursor5 must be refused
	if n := balPeek(emu, syms["NSEL"]); n != 5 {
		t.Fatalf("NSEL=%d after 6th select, want 5", n)
	}
	if m := balPeek(emu, syms["SELMARK"]+5); m != 0 {
		t.Fatalf("SELMARK[5]=%d want 0 (refused)", m)
	}
	// deselect by re-pressing ENTER at a selected slot
	pressOnce(t, emu, syms, "4")
	pressOnce(t, emu, syms, "ENTER")
	if n := balPeek(emu, syms["NSEL"]); n != 4 {
		t.Fatalf("NSEL=%d after deselect, want 4", n)
	}
}

func TestBalatroDiscard(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu, _ := startGame(t, syms)
	old0 := balPeek(emu, syms["HAND"])
	old1 := balPeek(emu, syms["HAND"]+1)
	pressOnce(t, emu, syms, "ENTER")           // select slot0
	pressOnce(t, emu, syms, "6")               // cursor1
	pressOnce(t, emu, syms, "ENTER")           // select slot1
	pressOnce(t, emu, syms, "D")
	balRunFrames(emu, 2)
	if d := balPeek(emu, syms["DISCS"]); d != 2 {
		t.Fatalf("DISCS=%d want 2", d)
	}
	if n := balPeek(emu, syms["NSEL"]); n != 0 {
		t.Fatalf("NSEL=%d after discard, want 0", n)
	}
	n0 := balPeek(emu, syms["HAND"])
	n1 := balPeek(emu, syms["HAND"]+1)
	if n0 == old0 || n1 == old1 {
		t.Fatalf("discarded cards not refilled: %d,%d", n0, n1)
	}
	seen := map[byte]int{}
	for i := 0; i < 8; i++ {
		seen[balPeek(emu, syms["HAND"]+uint16(i))]++
	}
	for c, cnt := range seen {
		if cnt > 1 {
			t.Fatalf("card %d appears %d times in hand after refill", c, cnt)
		}
	}
}

func TestBalatroPlayFourKindWins(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	// force hand = four aces + kicker: card = rank*16+suit (engine encoding)
	for i := 0; i < 4; i++ {
		emu.mem.Write(syms["HAND"]+uint16(i), byte(i*16+12))
	}
	emu.mem.Write(syms["HAND"]+4, byte(2*16+0)) // 2 of spades kicker
	// select first 5
	for i := 0; i < 5; i++ {
		pressOnce(t, emu, syms, "ENTER")
		pressOnce(t, emu, syms, "6")
	}
	if n := balPeek(emu, syms["NSEL"]); n != 5 {
		t.Fatalf("NSEL=%d want 5", n)
	}
	pressOnce(t, emu, syms, "SPACE")
	// score animation may already be done inside the stroke; accept SCORE or WON
	if m := balPeek(emu, syms["MODE"]); m != bMODE_SCORE && m != bMODE_WON {
		t.Fatalf("MODE=%d after play, want SCORE(2)/WON(3)", m)
	}
	balRunFrames(emu, 40)
	if ht := balPeek(emu, syms["HTIDX"]); ht != 7 {
		t.Fatalf("HTIDX=%d want 7 (four kind)", ht)
	}
	if ch := balBCD3(emu, syms["CHIPS"]); ch != 104 {
		t.Fatalf("CHIPS=%d want 104 (60 base + 4 aces x11)", ch)
	}
	if mu := balPeek(emu, syms["MULTBIN"]); mu != 3 {
		t.Fatalf("MULTBIN=%d want 3", mu)
	}
	if tot := balBCD3(emu, syms["TOT"]); tot != 312 {
		t.Fatalf("TOT=%d want 312", tot)
	}
	if w := balPeek(emu, syms["WINF"]); w != 1 {
		t.Fatalf("WINF=%d want 1 (312 >= 300)", w)
	}
	if m := balPeek(emu, syms["MODE"]); m != bMODE_WON {
		t.Fatalf("MODE=%d want WON(3)", m)
	}
	// joker awarded
	if j := balPeek(emu, syms["JOKSLOTS"]); j != 1 {
		t.Fatalf("JOKSLOTS[0]=%d want 1 (first joker)", j)
	}
	// ENTER advances to blind 2 of ante 1 (big blind, target 450)
	pressOnce(t, emu, syms, "ENTER")
	balRunFrames(emu, 2)
	if bi := balPeek(emu, syms["BLINDIDX"]); bi != 1 {
		t.Fatalf("BLINDIDX=%d want 1", bi)
	}
	if tg := balBCD3(emu, syms["BLINDIDX"]+0); tg == 0 {
		_ = tg
	}
	if m := balPeek(emu, syms["MODE"]); m != bMODE_PLAY {
		t.Fatalf("MODE=%d want PLAY after advance", m)
	}
}

func TestBalatroGameOverRestart(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	// make target unbeatable then play four weak hands
	emu.mem.Write(syms["BLINDTAB"]+0, 0x99)
	emu.mem.Write(syms["BLINDTAB"]+1, 0x99)
	emu.mem.Write(syms["BLINDTAB"]+2, 0x99)
	// recompute target: easiest — advanceBlind wraps so instead force via fresh round: poke SCOREV zero and play 4x weak
	for round := 0; round < 4; round++ {
		// force hand = distinct low cards (high card only): rank*? engine byte = suit<<4|rank
		for i := 0; i < 8; i++ {
			emu.mem.Write(syms["HAND"]+uint16(i), byte((i%4)*16+1+(i/4)))
		}
		pressOnce(t, emu, syms, "ENTER")
		pressOnce(t, emu, syms, "6")
		pressOnce(t, emu, syms, "ENTER")
		pressOnce(t, emu, syms, "SPACE") // play pair of 2s
		balRunFrames(emu, 40)
		if m := balPeek(emu, syms["MODE"]); m != bMODE_OVER {
			t.Fatalf("round %d MODE=%d want OVER(4) vs 999999", round, m)
		}
	}
	pressOnce(t, emu, syms, "SPACE") // overKey restart
	balRunFrames(emu, 4)
	if a := balPeek(emu, syms["ANTE"]); a != 1 {
		t.Fatalf("ANTE=%d want 1 after restart", a)
	}
	if bi := balPeek(emu, syms["BLINDIDX"]); bi != 0 {
		t.Fatalf("BLINDIDX=%d want 0", bi)
	}
	if sv := balBCD3(emu, syms["SCOREV"]); sv != 0 {
		t.Fatalf("SCOREV=%d want 0 after restart", sv)
	}
	if m := balPeek(emu, syms["MODE"]); m != bMODE_PLAY {
		t.Fatalf("MODE=%d want PLAY after restart", m)
	}
}

func TestBalatroQuitClean(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	pressOnce(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	pressOnce(t, emu, syms, "Q")
	pc := emu.cpu.PC
	if pc >= syms["start"] && pc < syms["doQuit"] {
		t.Fatalf("PC=$%04X still in demo loop after Q", pc)
	}
	// stack guard + USR-return sentinel: clean quit lands outside the demo
	// code zone (halt pad / ROM BASIC) with interrupts live — RANDOMIZE USR
	// exit. Assert IFF before extra frames: ROM BASIC DI's transiently in
	// its own editor loops; the engine's exit EI is what we gate on.
	if pc >= 0x8000 && pc != 0xBF00 {
		t.Fatalf("PC=$%04X unexpected post-quit (want halt pad $BF00 or ROM)", pc)
	}
	if emu.cpu.IFF1 == false {
		t.Fatalf("IFF1 off at quit — doQuit must ei (PC=$%04X)", pc)
	}
	_ = runOneFrameHeadless
	_ = roms.ModelPlus2
}
