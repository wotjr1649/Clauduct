package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The Codex CLI keeps the account's model catalogue in models_cache.json, refetched when
// Codex runs (codex-rs models-manager). New models, new efforts and retirements show up there
// before this build knows about them. doctor reports the difference; adopting a model is
// still a measurement and a table change, never something read off this file at runtime.

// codexCatalogue is the part of the cache doctor decodes. The cache may also name the
// signed-in account; no field here reaches it, so it is never decoded or printed.
type codexCatalogue struct {
	FetchedAt     time.Time      `json:"fetched_at"`
	ClientVersion string         `json:"client_version"`
	Models        []catalogModel `json:"models"`
}

type catalogModel struct {
	Slug           string `json:"slug"`
	Visibility     string `json:"visibility"`
	SupportedInAPI bool   `json:"supported_in_api"`
	Efforts        []struct {
		Effort string `json:"effort"`
	} `json:"supported_reasoning_levels"`
	ContextWindow    int64    `json:"context_window"`
	MaxContextWindow int64    `json:"max_context_window"`
	EffectivePercent int64    `json:"effective_context_window_percent"`
	InputModalities  []string `json:"input_modalities"`
	Upgrade          *struct {
		Model        string `json:"model"`
		RetirementAt string `json:"retirement_at"`
	} `json:"upgrade"`
}

// catalogue reads the cache from the OS user's Codex home, the one whose credential a
// session uses (auth.OSCodexHome ignores CODEX_HOME for the same reason).
func catalogue(out io.Writer) {
	raw, err := readCatalogue()
	installed, versionErr := upstream.InstalledVersion()()
	if versionErr != nil {
		installed = "unavailable"
	}
	catalogueReport(out, raw, err, installed, time.Now())
}

func readCatalogue() ([]byte, error) {
	home, err := auth.OSCodexHome()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(home, "models_cache.json"))
}

func decodeCatalogue(raw []byte) (cache codexCatalogue, ok bool) {
	return cache, json.Unmarshal(raw, &cache) == nil && !cache.FetchedAt.IsZero() && len(cache.Models) > 0
}

// catalogueReport never fails doctor: the cache is a hint about the backend, and a session
// does not need it.
func catalogueReport(out io.Writer, raw []byte, readErr error, installed string, now time.Time) {
	cache, decoded := decodeCatalogue(raw)
	switch {
	case errors.Is(readErr, fs.ErrNotExist):
		fmt.Fprintln(out, "models       unavailable: no Codex model cache; running codex once writes it")
		return
	case readErr != nil:
		fmt.Fprintln(out, "models       unavailable: the Codex model cache could not be read")
		return
	case !decoded:
		fmt.Fprintln(out, "models       unavailable: the Codex model cache is in a format this build does not recognise")
		return
	}
	fmt.Fprintf(out, "models       Codex cache from %s, %s old: only as fresh as the last Codex use\n",
		cache.FetchedAt.UTC().Format(time.RFC3339), now.Sub(cache.FetchedAt).Round(time.Minute))
	versions := "cache written by codex " + cache.ClientVersion + ", installed codex " + installed
	if cache.ClientVersion != installed {
		versions += " (differ: requests carry the installed version, and new models can need a newer one)"
	}
	fmt.Fprintln(out, "models       "+versions)

	listed := map[string]catalogModel{}
	for _, entry := range cache.Models {
		listed[entry.Slug] = entry
	}
	for _, model := range bridge.Models {
		entry, found := listed[model.ID]
		if !found {
			fmt.Fprintf(out, "models       %s: routed here, absent from the backend catalogue\n", model.ID)
			continue
		}
		if entry.Visibility != "list" {
			fmt.Fprintf(out, "models       %s: routed here, hidden by the backend\n", model.ID)
		}
		if entry.Upgrade != nil && entry.Upgrade.Model != "" {
			fmt.Fprintf(out, "models       %s: the backend names a successor, %s, retiring %s\n", model.ID, entry.Upgrade.Model, orUnknown(entry.Upgrade.RetirementAt))
		}
		offered := efforts(entry)
		if extra := without(offered, model.Efforts); len(extra) > 0 {
			fmt.Fprintf(out, "models       %s: the backend offers %s, not routed here\n", model.ID, strings.Join(extra, ", "))
		}
		if missing := without(model.Efforts, offered); len(missing) > 0 {
			fmt.Fprintf(out, "models       %s: routed here at %s, not offered by the backend\n", model.ID, strings.Join(missing, ", "))
		}
		if entry.ContextWindow != model.Context.Window {
			fmt.Fprintf(out, "models       %s: context %d here, backend %d (max %d)\n", model.ID, model.Context.Window, entry.ContextWindow, entry.MaxContextWindow)
		}
	}
	for _, entry := range cache.Models {
		routed := slices.ContainsFunc(bridge.Models, func(m bridge.Model) bool { return m.ID == entry.Slug })
		if entry.Visibility == "list" && entry.SupportedInAPI && !routed {
			fmt.Fprintf(out, "models       %s: listed by the backend, not routed here (efforts %s, context %d, max %d)\n",
				entry.Slug, strings.Join(efforts(entry), ", "), entry.ContextWindow, entry.MaxContextWindow)
		}
	}
}

func efforts(entry catalogModel) []string {
	out := make([]string, 0, len(entry.Efforts))
	for _, level := range entry.Efforts {
		out = append(out, level.Effort)
	}
	return out
}

func without(these, those []string) (out []string) {
	for _, item := range these {
		if !slices.Contains(those, item) {
			out = append(out, item)
		}
	}
	return out
}

func orUnknown(s string) string {
	if s == "" {
		return "date unknown"
	}
	return s
}
