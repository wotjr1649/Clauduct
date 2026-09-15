package upstream

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func approved() Budget { return Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 3} }

func reserve(t *testing.T, l *Ledger) error {
	t.Helper()
	return l.Reserve("gpt-5.6-luna", "low", false)
}

// LIFE08: the cap is a limit, not a report. The attempt after the last one is refused.
func TestTheCapStopsAtTheApprovedCount(t *testing.T) {
	ledger := NewLedger(approved())
	for i := 0; i < 3; i++ {
		if err := reserve(t, ledger); err != nil {
			t.Fatalf("attempt %d refused inside the budget: %v", i+1, err)
		}
	}
	if err := reserve(t, ledger); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("attempt 4 = %v, want %v", err, ErrBudgetExhausted)
	}
	// And it stays refused. A cap that lets the next one through has not capped anything.
	if err := reserve(t, ledger); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("attempt 5 = %v, want %v", err, ErrBudgetExhausted)
	}

	attempts, inferences, refused := ledger.Spent()
	if attempts != 3 || inferences != 3 || refused != 2 {
		t.Fatalf("spent = %d/%d/%d, want 3/3/2", attempts, inferences, refused)
	}
	if ledger.Remaining() != 0 {
		t.Fatalf("Remaining = %d, want 0", ledger.Remaining())
	}
}

// LIFE08: a retry costs an attempt but is not a second inference. A cap stated in attempts
// and measured in inferences would let a retry loop spend past it.
func TestARetryCostsAnAttemptButNotAnInference(t *testing.T) {
	ledger := NewLedger(approved())
	if err := ledger.Reserve("gpt-5.6-luna", "low", false); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	if err := ledger.Reserve("gpt-5.6-luna", "low", true); err != nil {
		t.Fatalf("retry: %v", err)
	}
	attempts, inferences, _ := ledger.Spent()
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2: a retry reached the backend and must be counted", attempts)
	}
	if inferences != 1 {
		t.Fatalf("inferences = %d, want 1: a retry answers the same question", inferences)
	}
}

// A budget authorises one route. The cheapest model at the lowest effort and the most
// expensive at the highest cost very different amounts for the same number of calls, so a
// count alone authorises nothing.
func TestOnlyTheApprovedRouteIsAllowed(t *testing.T) {
	for name, route := range map[string]struct{ model, effort string }{
		"a different model":     {"gpt-6-astra", "low"},
		"a different effort":    {"gpt-5.6-luna", "high"},
		"both different":        {"gpt-6-astra", "high"},
		"an empty model":        {"", "low"},
		"an empty effort":       {"gpt-5.6-luna", ""},
		"a case variant model":  {"GPT-5.6-Luna", "low"},
		"a case variant effort": {"gpt-5.6-luna", "Low"},
	} {
		t.Run(name, func(t *testing.T) {
			ledger := NewLedger(approved())
			err := ledger.Reserve(route.model, route.effort, false)
			if !errors.Is(err, ErrRouteNotAuthorised) {
				t.Fatalf("Reserve(%q, %q) = %v, want %v", route.model, route.effort, err, ErrRouteNotAuthorised)
			}
			if attempts, _, _ := ledger.Spent(); attempts != 0 {
				t.Fatalf("a refused route spent %d attempts, want 0", attempts)
			}
		})
	}
}

// REL12: the default authorises nothing. Live calls are not something to fall into by
// running the wrong command, so an unset budget refuses rather than defaulting to a small
// number of calls.
func TestAZeroBudgetAuthorisesNothing(t *testing.T) {
	for name, budget := range map[string]Budget{
		"entirely unset":   {},
		"no limit":         {Model: "gpt-5.6-luna", Effort: "low"},
		"a zero limit":     {Model: "gpt-5.6-luna", Effort: "low", Limit: 0},
		"a negative limit": {Model: "gpt-5.6-luna", Effort: "low", Limit: -1},
		"no model":         {Effort: "low", Limit: 10},
		"no effort":        {Model: "gpt-5.6-luna", Limit: 10},
	} {
		t.Run(name, func(t *testing.T) {
			ledger := NewLedger(budget)
			if err := reserve(t, ledger); !errors.Is(err, ErrBudgetExhausted) {
				t.Fatalf("Reserve on %+v = %v, want %v", budget, err, ErrBudgetExhausted)
			}
			if attempts, inferences, _ := ledger.Spent(); attempts != 0 || inferences != 0 {
				t.Fatalf("an unauthorised ledger spent %d attempts and %d inferences, want 0/0",
					attempts, inferences)
			}
			if ledger.Remaining() != 0 {
				t.Fatalf("Remaining = %d, want 0", ledger.Remaining())
			}
		})
	}
}

// The approved policy is the one the user authorised. A test states it so that changing it
// is a visible edit rather than a quiet one.
func TestTheApprovedBudgetIsTheOneThatWasAuthorised(t *testing.T) {
	got := ApprovedBudget()
	want := Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 100}
	if got != want {
		t.Fatalf("ApprovedBudget() = %+v, want %+v — changing this is a decision, not a fix", got, want)
	}
	// The count is the user's to raise. The route is the part they said to ask about, so it
	// is stated separately: a model change must fail this even if the count is right.
	if got.Model != "gpt-5.6-luna" || got.Effort != "low" {
		t.Fatalf("route = %s/%s. astra is the most expensive model and needs asking first.",
			got.Model, got.Effort)
	}
}

// Concurrent callers must not both pass the last slot. Run with -race.
func TestTheCapHoldsUnderConcurrentReservation(t *testing.T) {
	const limit, callers = 10, 64
	ledger := NewLedger(Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: limit})

	var wg sync.WaitGroup
	granted := make(chan struct{}, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ledger.Reserve("gpt-5.6-luna", "low", false); err == nil {
				granted <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(granted)

	if n := len(granted); n != limit {
		t.Fatalf("%d callers were granted an attempt, want exactly %d", n, limit)
	}
	if attempts, _, refused := ledger.Spent(); attempts != limit || refused != callers-limit {
		t.Fatalf("spent = %d attempts / %d refused, want %d/%d",
			attempts, refused, limit, callers-limit)
	}
}

// A ledger is printed in reports. It must not be a way to learn anything but counts.
func TestTheLedgerPrintsCountsAndNothingElse(t *testing.T) {
	ledger := NewLedger(approved())
	reserve(t, ledger)
	printed := ledger.String()
	for _, want := range []string{"gpt-5.6-luna/low", "attempts:1/3", "inferences:1", "refused:0"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("String() = %q, missing %q", printed, want)
		}
	}
}
