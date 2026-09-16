package upstream

import (
	"context"
	"testing"
)

// CAP03 and H09. What a session was billed for is recorded apart from what it asked for.
//
// This used to be recorded as nothing at all. The route lived on the transport as two
// fixed fields, and the product transport was built with both of them empty, so every
// attempt in a real session was reserved against a blank route. The attempts were counted;
// what they were spent on was not.
func TestTheLedgerRecordsWhatWasAskedForAndWhatRan(t *testing.T) {
	ledger := NewLedger(Unlimited())
	transport := &Direct{Ledger: ledger, Credentials: nil}

	// One session, three requests, three different routes. This is ordinary: a single
	// `claude -p` run sends the conversation and a session title, and the client does not
	// use the same model for both.
	for _, c := range []Call{
		{Requested: "claude-opus-5", Model: "gpt-5.6-sol", Effort: "xhigh", Source: "family"},
		{Requested: "haiku", Model: "gpt-5.6-luna", Effort: "max", Source: "alias"},
		{Requested: "claude-opus-5", Model: "gpt-5.6-sol", Effort: "xhigh", Source: "family"},
	} {
		c.Body = []byte(`{"model":"` + c.Model + `","reasoning":{"effort":"` + c.Effort + `"}}`)
		// Credentials are nil, so this stops right after the reservation. The reservation
		// is the part being measured: it happens before anything is dialled.
		_, _ = transport.Execute(context.Background(), c)
	}

	routes := ledger.Routes()
	if len(routes) != 2 {
		t.Fatalf("Routes() = %v, want the two distinct routes this session used", routes)
	}
	want := map[RouteRecord]int{
		{Requested: "claude-opus-5", Model: "gpt-5.6-sol", Effort: "xhigh", Source: "family"}: 2,
		{Requested: "haiku", Model: "gpt-5.6-luna", Effort: "max", Source: "alias"}:           1,
	}
	for route, attempts := range want {
		if got := ledger.Attempts(route); got != attempts {
			t.Fatalf("Attempts(%v) = %d, want %d. Recorded: %v", route, got, attempts, routes)
		}
	}
	// The requested name is kept, not overwritten by what it resolved to. A record that
	// only holds the effective model cannot answer whether the user got what they asked
	// for -- which is the whole question.
	if routes[0].Requested == routes[0].Model {
		t.Fatalf("Routes()[0] = %v: requested and effective collapsed into one value", routes[0])
	}
}

// And the authorisation is against what will actually be sent, not against a claim beside
// it. A caller that names an approved route while sending a different one would otherwise
// spend on a model nobody approved.
func TestADeclaredRouteThatTheBodyContradictsIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		call Call
	}{
		{"a cheaper model claimed than sent", Call{
			Body:   []byte(`{"model":"gpt-6-astra","reasoning":{"effort":"medium"}}`),
			Model:  "gpt-5.6-luna",
			Effort: "medium",
		}},
		{"a lower effort claimed than sent", Call{
			Body:   []byte(`{"model":"gpt-5.6-luna","reasoning":{"effort":"max"}}`),
			Model:  "gpt-5.6-luna",
			Effort: "low",
		}},
		{"no effort in the body at all", Call{
			Body:   []byte(`{"model":"gpt-5.6-luna"}`),
			Model:  "gpt-5.6-luna",
			Effort: "low",
		}},
		{"a body that is not a request", Call{
			Body:   []byte(`not json`),
			Model:  "gpt-5.6-luna",
			Effort: "low",
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ledger := NewLedger(ApprovedBudget())
			transport := &Direct{Ledger: ledger}

			_, err := transport.Execute(context.Background(), c.call)
			if err != ErrRouteMismatch {
				t.Fatalf("Execute = %v, want %v", err, ErrRouteMismatch)
			}
			if attempts, _, _ := ledger.Spent(); attempts != 0 {
				t.Fatalf("the ledger spent %d attempts on a request that was never sent",
					attempts)
			}
		})
	}
}
