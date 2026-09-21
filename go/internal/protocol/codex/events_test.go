package codex

import "testing"

// Session 36, D. Every count the backend reports, not the two somebody thought to read.
//
// Measured 2026-09-18 against the real backend, four requests on gpt-5.6-luna at low effort:
// a response.completed carries input_tokens, output_tokens, total_tokens,
// input_tokens_details.cached_tokens, input_tokens_details.cache_write_tokens and
// output_tokens_details.reasoning_tokens. This decoder read the first two. cached_tokens read
// 3,840 on a 16,865-token prefix, so it is a measurement and not a field that is always zero,
// and reasoning tokens are billed and had never been counted at all.
func TestUsageCarriesEveryCountTheBackendReports(t *testing.T) {
	// The shape as it arrived, values from the 2026-09-18 run.
	usage, err := DecodeUsage([]byte(`{"type":"response.completed","response":{"usage":{` +
		`"input_tokens":16865,"input_tokens_details":{"cached_tokens":3840,"cache_write_tokens":0},` +
		`"output_tokens":5,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":16870}}}`))
	if err != nil {
		t.Fatalf("DecodeUsage: %v", err)
	}

	for _, c := range []struct {
		name  string
		got   int64
		known bool
		want  int64
	}{
		{"input", usage.InputTokens, usage.InputKnown, 16865},
		{"output", usage.OutputTokens, usage.OutputKnown, 5},
		{"cached input", usage.CachedInputTokens, usage.CachedInputKnown, 3840},
		{"cache write", usage.CacheWriteTokens, usage.CacheWriteKnown, 0},
		{"reasoning", usage.ReasoningTokens, usage.ReasoningKnown, 0},
		{"total", usage.TotalTokens, usage.TotalKnown, 16870},
	} {
		if !c.known {
			t.Errorf("%s tokens were reported and this reads them as unknown", c.name)
		}
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// Absent stays absent. A missing count reported as zero turns "we do not know" into "it cost
// nothing", which is exactly how the client's "0 cached" came to look like a measurement.
func TestAnAbsentCountIsUnknownRatherThanZero(t *testing.T) {
	usage, err := DecodeUsage([]byte(`{"type":"response.completed","response":{"usage":{` +
		`"input_tokens":5,"output_tokens":2}}}`))
	if err != nil {
		t.Fatalf("DecodeUsage: %v", err)
	}
	if usage.CachedInputKnown || usage.CacheWriteKnown || usage.ReasoningKnown || usage.TotalKnown {
		t.Errorf("a usage object with none of these claims to know them: %+v", usage)
	}
	if !usage.InputKnown || !usage.OutputKnown {
		t.Errorf("the two counts that were there read as unknown: %+v", usage)
	}
}

// A details member that is not an object, or holds something that is not a count, leaves the
// count unknown rather than failing the whole read. Losing input_tokens because a sibling
// changed shape would be a worse answer than losing the sibling.
func TestAMalformedDetailDoesNotCostTheCountsBesideIt(t *testing.T) {
	for name, raw := range map[string]string{
		"details is not an object": `{"response":{"usage":{"input_tokens":5,` +
			`"input_tokens_details":"none","output_tokens":2}}}`,
		"the count is not a number": `{"response":{"usage":{"input_tokens":5,` +
			`"input_tokens_details":{"cached_tokens":"lots"},"output_tokens":2}}}`,
		"details is null": `{"response":{"usage":{"input_tokens":5,` +
			`"input_tokens_details":null,"output_tokens":2}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			usage, err := DecodeUsage([]byte(raw))
			if err != nil {
				t.Fatalf("DecodeUsage: %v", err)
			}
			if !usage.InputKnown || usage.InputTokens != 5 || !usage.OutputKnown {
				t.Errorf("the counts beside it were lost: %+v", usage)
			}
			if usage.CachedInputKnown {
				t.Errorf("an unreadable detail was reported as known: %+v", usage)
			}
		})
	}
}
