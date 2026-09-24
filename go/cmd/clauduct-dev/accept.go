package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// accept measures one backend model before it enters the routing table (#108): whether each
// effort the Codex catalogue lists is accepted and what it uses, which events and output
// items the stream carries, the tool and reasoning round trips, image input, and whether the
// local token count agrees with the backend. The report is labels, counts and timings.
//
// Adding a model is this report and a table change. Nothing here routes anything: the model
// is sent as a route override, so the table stays what it was.

// acceptPlan is what a run will send, fixed before anything is sent.
type acceptPlan struct {
	model   string
	efforts []string
	// full adds a tool call, its result, the reasoning record sent back and (when the
	// catalogue lists image input) an image, all at the cheapest effort.
	full  bool
	image bool
	entry catalogModel
}

// attempts is the ledger cap for one effort's route.
func (p acceptPlan) attempts(effort string) int {
	if !p.full || effort != p.efforts[0] {
		return 1
	}
	if p.image {
		return 5
	}
	return 4
}

func (p acceptPlan) total() (n int) {
	for _, effort := range p.efforts {
		n += p.attempts(effort)
	}
	return n
}

// planAccept takes the efforts from the catalogue. Naming efforts runs only those, one
// request each, which is how a single effort such as ultra is measured on its own.
func planAccept(cache codexCatalogue, model string, only []string) (acceptPlan, error) {
	i := slices.IndexFunc(cache.Models, func(m catalogModel) bool { return m.Slug == model })
	if i < 0 {
		return acceptPlan{}, errors.New("MODEL_NOT_IN_CATALOGUE")
	}
	entry := cache.Models[i]
	offered := efforts(entry)
	plan := acceptPlan{model: model, efforts: offered, full: len(only) == 0, image: slices.Contains(entry.InputModalities, "image"), entry: entry}
	if len(offered) == 0 {
		return acceptPlan{}, errors.New("NO_CATALOGUE_EFFORTS")
	}
	if len(only) > 0 {
		for _, effort := range only {
			if !slices.Contains(offered, effort) {
				return acceptPlan{}, errors.New("EFFORT_NOT_OFFERED")
			}
		}
		plan.efforts = slices.DeleteFunc(slices.Clone(offered), func(e string) bool { return !slices.Contains(only, e) })
	}
	return plan, nil
}

func (p acceptPlan) print(out io.Writer) {
	fmt.Fprintf(out, "accept  %s, efforts from the Codex catalogue: %s\n", p.model, strings.Join(p.efforts, ", "))
	for _, effort := range p.efforts {
		fmt.Fprintf(out, "        %s/%s: %d attempts\n", p.model, effort, p.attempts(effort))
	}
	fmt.Fprintf(out, "        total %d attempts; each route is capped by its own ledger before any socket opens\n", p.total())
}

func acceptCommand(args []string, out, errOut io.Writer) int {
	send := len(args) > 0 && args[len(args)-1] == "--send"
	if send {
		args = args[:len(args)-1]
	}
	if len(args) == 0 || slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "-") }) {
		return usageProbe(out)
	}
	raw, err := readCatalogue()
	cache, decoded := decodeCatalogue(raw)
	if err != nil || !decoded {
		fmt.Fprintln(errOut, "probe   REFUSED the Codex model cache is unavailable, so the model's efforts are unknown")
		return 1
	}
	plan, err := planAccept(cache, args[0], args[1:])
	if err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "probe sends real requests and spends real money. Nothing is sent without --send.")
	plan.print(out)
	if !send {
		return 2
	}

	provider := &auth.Provider{}
	if err := provider.CheckRuntime(); err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %s\n", auth.CategoryOf(err))
		return 1
	}
	if err := provider.CheckHome(); err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %s\n", auth.CategoryOf(err))
		return 1
	}
	version, err := upstream.InstalledVersion()()
	if err != nil {
		fmt.Fprintf(errOut, "probe   REFUSED %s\n", err)
		return 1
	}
	fmt.Fprintf(out, "client  codex-cli %s (%s)\n", version, upstream.Status(version))

	var ledgers []*upstream.Ledger
	code := acceptProbe(plan, func(effort string) upstream.Transport {
		ledger := upstream.NewLedger(upstream.Budget{Model: plan.model, Effort: effort, Limit: plan.attempts(effort)})
		ledgers = append(ledgers, ledger)
		return upstream.NewDirect(provider, ledger, upstream.Fixed(version))
	}, out)
	var attempts, inferences, refused int
	for _, ledger := range ledgers {
		a, i, r := ledger.Spent()
		attempts, inferences, refused = attempts+a, inferences+i, refused+r
	}
	fmt.Fprintf(out, "spent   %d attempts, %d inferences, %d refused\n", attempts, inferences, refused)
	return code
}

// acceptProbe runs the plan. open returns the transport for one effort's route.
func acceptProbe(plan acceptPlan, open func(effort string) upstream.Transport, out io.Writer) int {
	transports := map[string]upstream.Transport{}
	for _, effort := range plan.efforts {
		transports[effort] = open(effort)
	}
	// The request names a table model only so it decodes; the override is what is sent.
	head := fmt.Sprintf(`{"model":%q,"max_tokens":2048,"stream":true,"system":"You are a verification fixture. Be brief.",`, bridge.Models[0].ID)
	events, items := map[string]int{}, map[string]int{}
	failed, counted, matched := 0, 0, 0
	run := func(label, effort, body string) exchange {
		start := time.Now()
		result := routedTrip(transports[effort], out, label, body, []bridge.Route{{Model: plan.model, Effort: effort, Source: "probe"}})
		elapsed := time.Since(start).Round(time.Millisecond)
		if !result.ok {
			failed++
		}
		for name, n := range result.eventCounts {
			events[name] += n
		}
		for name, n := range result.itemCounts {
			items[name] += n
		}
		line := fmt.Sprintf("               %s; events %s; items %s; usage %s", elapsed, countList(result.eventCounts), countList(result.itemCounts), fieldList(result.usage))
		if local, ok := localCount(result.request); ok && result.usage["input_tokens"] > 0 {
			counted++
			if local == result.usage["input_tokens"] {
				matched++
			}
			line += fmt.Sprintf("; count local %d backend %d", local, result.usage["input_tokens"])
		}
		fmt.Fprintln(out, line)
		return result
	}

	var accepted, refused []string
	var thought string
	var text exchange
	for i, effort := range plan.efforts {
		result := run("effort "+effort, effort, head+`"messages":[{"role":"user","content":"Reply with the word ok."}]}`)
		if i == 0 {
			text = result
		}
		if result.ok {
			accepted = append(accepted, effort)
		} else {
			refused = append(refused, effort)
		}
		if thought == "" {
			thought = result.thought
		}
	}

	var readings []string
	if plan.full {
		low := plan.efforts[0]
		call := run("tool call", low, head+`"tools":[`+probeTool+`],"tool_choice":{"type":"tool","name":"get_build_token"},`+
			`"messages":[{"role":"user","content":`+quote(probeToolPrompt)+`}]}`)
		var result exchange
		if call.ok && call.callID != "" {
			result = run("tool result", low, toolHistory(head, call))
		} else {
			fmt.Fprintln(out, "tool result    NOT_RUN — no tool call came back")
		}
		readings = append(readings, "tools "+wireVerdict(text, call, result))
		if thought == "" {
			thought = call.thought
		}

		switch {
		case thought == "":
			fmt.Fprintln(out, "reasoning back NOT_RUN — no turn came back with an encrypted record")
			readings = append(readings, "reasoning NOT_RUN")
		default:
			back := run("reasoning back", low, head+`"messages":[`+
				`{"role":"user","content":"Name one prime between 10 and 20. One word."},`+
				`{"role":"assistant","content":[{"type":"redacted_thinking","data":`+quote(thought)+`}]},`+
				`{"role":"user","content":"And one between 20 and 30. One word."}]}`)
			readings = append(readings, "reasoning "+passFail(back.ok))
		}

		if plan.image {
			image := run("image", low, head+`"messages":[{"role":"user","content":[`+
				`{"type":"text","text":"Reply with the single word: image"},`+
				`{"type":"image","source":{"type":"base64","media_type":"image/png","data":`+quote(onePixelPNG)+`}}]}]}`)
			readings = append(readings, "image "+passFail(image.ok))
		} else {
			readings = append(readings, "image NOT_RUN (not in the catalogue's input modalities)")
		}
	}

	fmt.Fprintf(out, "events   %s\n", countList(events))
	fmt.Fprintf(out, "items    %s\n", countList(items))
	fmt.Fprintf(out, "count    %d of %d text requests matched the backend's input_tokens exactly\n", matched, counted)
	fmt.Fprintf(out, "context  catalogue window %d, max %d, effective %d%%\n", plan.entry.ContextWindow, plan.entry.MaxContextWindow, plan.entry.EffectivePercent)
	fmt.Fprintf(out, "reading  efforts accepted [%s] refused [%s]", strings.Join(accepted, " "), strings.Join(refused, " "))
	for _, reading := range readings {
		fmt.Fprintf(out, "; %s", reading)
	}
	fmt.Fprintln(out)
	if failed > 0 {
		return 1
	}
	return 0
}

// localCount applies the local formula as though the model were already validated. The
// comparison with the backend is what decides whether it may be (CountValidated).
func localCount(request *bridge.Request) (int64, bool) {
	if request == nil {
		return 0, false
	}
	for _, model := range bridge.Models {
		if model.CountValidated {
			probe := *request
			probe.Model = model.ID
			n, err := bridge.CountInput(&probe)
			return n, err == nil
		}
	}
	return 0, false
}

func countList(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, counts[name]))
	}
	return strings.Join(parts, " ")
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
