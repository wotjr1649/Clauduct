package upstream

import (
	"errors"
	"fmt"
	"sync"
)

// Refusals the ledger produces. Each one is a different reason a request will not be sent,
// and they are separate because they lead to different actions.
var (
	// ErrBudgetExhausted means the cap has been reached. Raising it is a decision, not a
	// retry.
	ErrBudgetExhausted = errors.New("REQUEST_BUDGET")
	// ErrRouteNotAuthorised means the model or effort is outside what was approved. A
	// budget authorises a specific route, not spending in general.
	ErrRouteNotAuthorised = errors.New("ROUTE_NOT_AUTHORISED")
)

// Budget is what a live run is allowed to spend.
//
// A zero Budget authorises nothing, which is the default everywhere. Live calls are not
// something to fall into by running the wrong command.
type Budget struct {
	// Model and Effort name the one route that may be used. A budget authorises a route,
	// not an amount: the cheapest model at the lowest effort and the most expensive at the
	// highest cost very different amounts for the same count.
	Model  string
	Effort string
	// Limit is the cumulative ceiling on attempts for the whole process.
	Limit int
	// Unrestricted authorises any route and any number of attempts.
	//
	// This is what a product session runs under, and it is a different kind of thing from
	// the verification budget above. That one exists to stop this project from spending a
	// person's money while developing against their account. A session the user started
	// themselves is them using the product on their own subscription, and capping it would
	// be inventing a restriction the Node baseline does not have -- a long session would
	// simply stop working partway through.
	//
	// Pinning a route would be worse than useless here: the model comes from the client's
	// request, so a fixed route would refuse every model but one.
	//
	// It is a separate field rather than a sentinel value so that it cannot be arrived at
	// by arithmetic. A zero Budget still authorises nothing.
	Unrestricted bool
}

// Unlimited is the budget a product session runs under. Named so that granting it is a
// visible act at the call site rather than a struct literal nobody reads.
func Unlimited() Budget { return Budget{Unrestricted: true} }

// authorises reports whether this budget permits anything at all. A budget missing any of
// its three parts authorises nothing: a count with no route names no price, and a route
// with no count names no ceiling.
func (b Budget) authorises() bool {
	if b.Unrestricted {
		return true
	}
	return b.Model != "" && b.Effort != "" && b.Limit > 0
}

// ApprovedBudget is the policy the user authorised.
//
// gpt-5.6-luna at low effort. luna is the cheapest of the four routes and low is its
// cheapest effort; gpt-6-astra is the top-tier model and the most expensive, so it is
// deliberately not the one a verification run spends on. Reasoning tokens count toward what
// a call costs, which is why the effort is pinned rather than left to the caller.
//
//	2026-09-15  twenty attempts
//	2026-09-15  raised to one hundred, route unchanged
//
// The route is the part that is not delegated. The user's standing instruction is to spend
// on luna without asking and to ask before astra, so a change of model is a new decision
// even though a change of count is not.
func ApprovedBudget() Budget {
	return Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 100}
}

// Ledger enforces a budget and records what was spent.
//
// Reservation happens before a socket is opened. Counting attempts after the fact would
// mean the cap is a report rather than a limit, and the thing being capped has already
// happened by the time the report is written.
type Ledger struct {
	budget Budget

	mu sync.Mutex
	// HTTP attempts and logical inferences are counted separately because they are
	// different numbers: one inference that is retried twice is one inference and three
	// attempts. A cap stated in one unit and measured in the other is not a cap.
	attempts   int
	inferences int
	refused    int
}

func NewLedger(budget Budget) *Ledger { return &Ledger{budget: budget} }

// Reserve claims one attempt on a route, before anything is dialled.
//
// retry says this attempt continues an inference already counted, so the inference is not
// counted twice. It still consumes an attempt: a retry costs a request whether or not it
// is a new question.
func (l *Ledger) Reserve(model, effort string, retry bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.budget.authorises() {
		l.refused++
		return ErrBudgetExhausted
	}
	if !l.budget.Unrestricted {
		if model != l.budget.Model || effort != l.budget.Effort {
			l.refused++
			return ErrRouteNotAuthorised
		}
		if l.attempts >= l.budget.Limit {
			l.refused++
			return ErrBudgetExhausted
		}
	}

	// Counted here rather than after the response, because the cost is the request, not
	// the answer. An attempt that fails, times out or is cancelled mid-flight still reached
	// the backend, so nothing is ever given back.
	l.attempts++
	if !retry {
		l.inferences++
	}
	return nil
}

// Spent reports the ledger. A budget claim rests on these numbers, so they are readable
// without inspecting anything that carries content.
func (l *Ledger) Spent() (attempts, inferences, refused int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts, l.inferences, l.refused
}

// Remaining reports how many attempts are left. It answers through the same condition
// Reserve uses, so a ledger that will refuse everything never reports attempts to spend.
func (l *Ledger) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.budget.authorises() {
		return 0
	}
	if l.budget.Unrestricted {
		// Not a number. Reporting a large one would invite a caller to treat it as a
		// ceiling, and there is not one.
		return -1
	}
	if l.budget.Limit <= l.attempts {
		return 0
	}
	return l.budget.Limit - l.attempts
}

func (l *Ledger) String() string {
	attempts, inferences, refused := l.Spent()
	if l.budget.Unrestricted {
		return fmt.Sprintf("upstream.Ledger{route:any attempts:%d inferences:%d refused:%d}",
			attempts, inferences, refused)
	}
	return fmt.Sprintf("upstream.Ledger{route:%s/%s attempts:%d/%d inferences:%d refused:%d}",
		l.budget.Model, l.budget.Effort, attempts, l.budget.Limit, inferences, refused)
}
