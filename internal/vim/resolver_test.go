package vim

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

func TestGGChord(t *testing.T) {
	km := keymap.Default()

	t.Run("g arms, second g fires", func(t *testing.T) {
		var r Resolver
		if got := r.GG(km.JumpTopPrefix, "g", true); got != ChordArmed {
			t.Fatalf("first g = %v, want ChordArmed", got)
		}
		if got := r.GG(km.JumpTopPrefix, "g", true); got != ChordFired {
			t.Fatalf("second g = %v, want ChordFired", got)
		}
	})

	t.Run("fired chord does not stay armed", func(t *testing.T) {
		var r Resolver
		r.GG(km.JumpTopPrefix, "g", true)
		r.GG(km.JumpTopPrefix, "g", true)
		if got := r.GG(km.JumpTopPrefix, "g", true); got != ChordArmed {
			t.Fatalf("g after a fired chord = %v, want ChordArmed (fresh chord)", got)
		}
	})

	// The preview and message viewers jump immediately on Home; bare g
	// keeps the chord.
	t.Run("home fires immediately when immediate", func(t *testing.T) {
		var r Resolver
		if got := r.GG(km.JumpTopPrefix, "home", true); got != ChordFired {
			t.Fatalf("home = %v, want ChordFired", got)
		}
	})

	// The file browser arms on any JumpTopPrefix key, Home included.
	t.Run("home arms when not immediate", func(t *testing.T) {
		var r Resolver
		if got := r.GG(km.JumpTopPrefix, "home", false); got != ChordArmed {
			t.Fatalf("home = %v, want ChordArmed", got)
		}
		if got := r.GG(km.JumpTopPrefix, "home", false); got != ChordFired {
			t.Fatalf("second home = %v, want ChordFired", got)
		}
	})

	// An armed g followed by a non-chord key clears the pending state
	// and lets the key fall through to normal handling — gg does not
	// swallow, unlike the z chord.
	t.Run("other key clears pending and falls through", func(t *testing.T) {
		var r Resolver
		r.GG(km.JumpTopPrefix, "g", true)
		if got := r.GG(km.JumpTopPrefix, "j", true); got != ChordNone {
			t.Fatalf("j while armed = %v, want ChordNone", got)
		}
		if got := r.GG(km.JumpTopPrefix, "g", true); got != ChordArmed {
			t.Fatalf("g after cleared chord = %v, want ChordArmed", got)
		}
	})

	t.Run("unrelated key with nothing armed", func(t *testing.T) {
		var r Resolver
		if got := r.GG(km.JumpTopPrefix, "x", true); got != ChordNone {
			t.Fatalf("x = %v, want ChordNone", got)
		}
	})
}

func TestScrollChord(t *testing.T) {
	km := keymap.Default()

	fire := func(t *testing.T, second string, want ScrollOp) {
		t.Helper()
		var r Resolver
		if got, _ := r.Scroll(km, "z"); got != ChordArmed {
			t.Fatalf("z = %v, want ChordArmed", got)
		}
		got, op := r.Scroll(km, second)
		if got != ChordFired {
			t.Fatalf("z%s = %v, want ChordFired", second, got)
		}
		if op != want {
			t.Fatalf("z%s op = %v, want %v", second, op, want)
		}
	}

	t.Run("zz centers", func(t *testing.T) { fire(t, "z", ScrollOpCenter) })
	t.Run("zt tops", func(t *testing.T) { fire(t, "t", ScrollOpTop) })
	t.Run("zb bottoms", func(t *testing.T) { fire(t, "b", ScrollOpBottom) })

	// A mistyped continuation is swallowed so it cannot trigger some
	// unrelated action bound to the second key.
	t.Run("unknown continuation is swallowed", func(t *testing.T) {
		var r Resolver
		r.Scroll(km, "z")
		if got, _ := r.Scroll(km, "q"); got != ChordSwallowed {
			t.Fatalf("zq = %v, want ChordSwallowed", got)
		}
		if got, _ := r.Scroll(km, "t"); got != ChordNone {
			t.Fatalf("t after swallowed chord = %v, want ChordNone (chord closed)", got)
		}
	})

	t.Run("unrelated key with nothing armed", func(t *testing.T) {
		var r Resolver
		if got, _ := r.Scroll(km, "j"); got != ChordNone {
			t.Fatalf("j = %v, want ChordNone", got)
		}
	})
}

// The two chords keep independent pending state, matching the separate
// flags they replace: arming z must not affect a later gg and vice versa.
func TestChordsAreIndependent(t *testing.T) {
	km := keymap.Default()
	var r Resolver

	r.Scroll(km, "z")
	if got := r.GG(km.JumpTopPrefix, "g", true); got != ChordArmed {
		t.Fatalf("g with z armed = %v, want ChordArmed", got)
	}
	if got, _ := r.Scroll(km, "t"); got != ChordFired {
		t.Fatalf("zt with g armed = %v, want ChordFired", got)
	}
}

func TestCountAccumulation(t *testing.T) {
	t.Run("digits accumulate", func(t *testing.T) {
		var r Resolver
		for _, d := range []string{"1", "2"} {
			if !r.Digit(d) {
				t.Fatalf("digit %q not consumed", d)
			}
		}
		if got := r.PendingCount(); got != 12 {
			t.Fatalf("PendingCount = %d, want 12", got)
		}
		if got := r.TakeCount(); got != 12 {
			t.Fatalf("TakeCount = %d, want 12", got)
		}
		if got := r.PendingCount(); got != 0 {
			t.Fatalf("PendingCount after take = %d, want 0", got)
		}
	})

	t.Run("take with nothing pending is one", func(t *testing.T) {
		var r Resolver
		if got := r.TakeCount(); got != 1 {
			t.Fatalf("TakeCount = %d, want 1", got)
		}
	})

	// A lone 0 is not a count starter — it stays free for the future
	// line-start motion.
	t.Run("lone zero is not consumed", func(t *testing.T) {
		var r Resolver
		if r.Digit("0") {
			t.Fatal("lone 0 was consumed")
		}
		if got := r.PendingCount(); got != 0 {
			t.Fatalf("PendingCount = %d, want 0", got)
		}
	})

	t.Run("zero continues a started count", func(t *testing.T) {
		var r Resolver
		r.Digit("1")
		if !r.Digit("0") {
			t.Fatal("0 after 1 not consumed")
		}
		if got := r.PendingCount(); got != 10 {
			t.Fatalf("PendingCount = %d, want 10", got)
		}
	})

	t.Run("non-digit is never consumed", func(t *testing.T) {
		var r Resolver
		r.Digit("3")
		for _, k := range []string{"j", "esc", "ctrl+d", "alt+3", "space"} {
			if r.Digit(k) {
				t.Fatalf("key %q consumed as digit", k)
			}
		}
	})

	t.Run("clear drops the count", func(t *testing.T) {
		var r Resolver
		r.Digit("4")
		r.ClearCount()
		if got := r.TakeCount(); got != 1 {
			t.Fatalf("TakeCount after clear = %d, want 1", got)
		}
	})

	t.Run("resolver clear drops the count too", func(t *testing.T) {
		var r Resolver
		r.Digit("4")
		r.Clear()
		if got := r.PendingCount(); got != 0 {
			t.Fatalf("PendingCount after Clear = %d, want 0", got)
		}
	})

	// A ridiculous count must not overflow into nonsense.
	t.Run("count is capped", func(t *testing.T) {
		var r Resolver
		for i := 0; i < 12; i++ {
			r.Digit("9")
		}
		if got := r.PendingCount(); got <= 0 || got > countCap {
			t.Fatalf("PendingCount = %d, want within (0, %d]", got, countCap)
		}
	})
}
