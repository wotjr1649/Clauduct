package providerprobe

import (
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// RESUME_TUI (#319 owner decision 16): a TUI launched with --resume <UUID> and no -p. A -p seed
// turn makes the session in the same world, so its transcript and stored profile exist; the TUI
// then resumes it.

// p3SeedTurn is the seed turn's canary ID in a resume-TUI case.
const p3SeedTurn = "turn0"

// tuiResumeCases are the RESUME_TUI scope's own cases.
func tuiResumeCases() map[string]func() scopeCase {
	storedA := func(w *p3World) error { return w.storeGateway(w.sgA) }
	return map[string]func() scopeCase{
		// R06-TUI: the resumed TUI turn is answered by INTENDED, its request carries the seed turn's
		// conversation, and it runs the seed's non-default model and effort (the stored profile
		// restored, as R07m shows for print).
		"R06-TUI": func() scopeCase {
			c := p3Case{ID: "R06-TUI", Entry: p3TUI, Hooks: p3HooksNormal, Setup: storedA, Expect: providercert.ExpectSuccess,
				TUIResume: p3SeedTurn, SeedPair: true, RestoreTurn: 1, Established: gotCanary("WRONG_STORED_GATEWAY_A", 1)}
			return scopeCase{c: c}
		},
	}
}

// resumeTUIScope makes every TUI background-entry check of the registry run in a resumed session:
// the same stimulus and judge, with the TUI launched by --resume of a seed session.
func resumeTUIScope(reg map[string]func() scopeCase) {
	for id, build := range reg {
		build := build
		reg[id] = func() scopeCase {
			sc := build()
			if sc.c.Entry == p3TUI && sc.c.TUIStimulus != nil && sc.c.TUIResume == "" {
				sc.c.TUIResume = p3SeedTurn
			}
			return sc
		}
	}
}
