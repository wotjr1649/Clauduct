package gateway

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func projectsFixture(t *testing.T, mode string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "projects")
	switch mode {
	case "absent":
	case "file", "file_parent":
		if err := os.WriteFile(path, []byte("public fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		if mode == "file_parent" {
			path = filepath.Join(path, "missing", "projects")
		}
	case "locked":
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		p, err := syscall.UTF16PtrFromString(path)
		if err != nil {
			t.Fatal(err)
		}
		h, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := syscall.CloseHandle(h); err != nil {
				t.Error(err)
			}
		})
	default:
		t.Fatal("unknown fixture")
	}
	return path
}

func TestProjectsAbsenceDiffersFromFileAndSharingFailure(t *testing.T) {
	for _, mode := range []string{"absent", "file", "file_parent", "locked"} {
		t.Run(mode, func(t *testing.T) {
			path := projectsFixture(t, mode)
			g := &Gateway{delegations: &delegations{projects: path}}
			g.EnableContextPolicy()
			g.contexts.sessions = map[string]string{"s": "project/s.jsonl"}
			binding := agentBinding{ID: "child", Role: "custom", SessionID: "s", TranscriptPath: filepath.Join(path, "project", "s.jsonl")}
			_, metaErr := g.delegations.metadata(binding)
			if (metaErr == errMetadataPending) != (mode == "absent") || mode != "absent" && !errors.Is(metaErr, errDelegationUnverified) {
				t.Fatal("filesystem failure became metadata retry", mode, metaErr)
			}
			contextErr := g.restoreContext("s", "", &contextState{})
			if (contextErr == nil) != (mode == "absent") {
				t.Fatal("filesystem failure became empty context history", mode, contextErr)
			}
			g.stripContextDisplays(&anthropic.Request{}, "s")
			wantUnreadable := int64(1)
			if mode == "absent" {
				wantUnreadable = 0
			}
			if got := g.contextDisplayReport().Unreadable; got != wantUnreadable {
				t.Fatal("wrong provenance failure count", mode, got)
			}
			_, found, choiceErr := g.delegations.loadChoice(delegationScope{session: "s"}, binding.ID, binding)
			if found || !errors.Is(choiceErr, errDelegationUnverified) {
				t.Fatal("missing root fell through to role routing", mode, choiceErr)
			}
		})
	}
}

func TestProjectsFailureDoesNotCountAsAnUnroutedRole(t *testing.T) {
	for _, mode := range []string{"absent", "file", "file_parent", "locked", "missing_choice"} {
		t.Run(mode, func(t *testing.T) {
			fixture := &upstream.Fixture{}
			g := startWith(t, fixture)
			g.EnableContextPolicy()
			var path string
			if mode == "absent" || mode == "missing_choice" {
				path = filepath.Join(t.TempDir(), "projects")
				g.ConfigureDelegations(path)
				if mode == "absent" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				path = projectsFixture(t, mode)
				g.ConfigureDelegations(path)
			}
			binding := agentBinding{ID: "child", Role: "custom", SessionID: "s", TranscriptPath: filepath.Join(path, "project", "s.jsonl")}
			if _, err := g.agents.register(binding, time.Now()); err != nil {
				t.Fatal(err)
			}
			r := messages(strings.NewReader(`{"model":"gpt-5.6-sol","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"public"}]}`))
			r.headers["X-Claude-Code-Session-Id"] = "s"
			r.headers["X-Claude-Code-Agent-Id"] = "child"
			r.headers["X-Claude-Code-Request-Class"] = "subagent"
			response := do(t, g, r)
			body := bodyText(t, response)
			if response.StatusCode != http.StatusBadRequest || !strings.Contains(body, "AGENT_SELECTION_UNVERIFIED") || fixture.Calls() != 0 {
				t.Fatal("unverified selection reached inference", response.StatusCode, body)
			}
			want := int64(0)
			if mode == "missing_choice" {
				want = 1
			}
			unregistered, unrouted := g.Unrouted()
			if unregistered != 0 || unrouted != want {
				t.Fatal("filesystem failure polluted role counters", mode, unregistered, unrouted)
			}
			if (mode == "file" || mode == "file_parent") && g.Snapshot().Agents.ProjectsUnavailable == "" {
				t.Fatal("filesystem setup diagnostic lost")
			}
		})
	}
}

func TestProjectsOpenerRejectsEscapesBeforeAbsenceAndRechecksRepairs(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-created", "projects")
	d := &delegations{projects: missing}
	for _, path := range []string{"../escape", filepath.Join(t.TempDir(), "absolute"), "NUL"} {
		root, err := d.openProjects(path)
		if root != nil {
			root.Close()
			t.Fatal("escape opened a root")
		}
		if !errors.Is(err, errProjectsEscape) {
			t.Fatal("escape mistaken for absence", err)
		}
	}
	root, err := d.openProjects("project/s.jsonl")
	if root != nil {
		root.Close()
		t.Fatal("missing root created by a read")
	}
	if !errors.Is(err, errProjectsAbsent) {
		t.Fatal("true absence not classified", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("opener changed the tree")
	}
	file := projectsFixture(t, "file")
	g := &Gateway{}
	g.ConfigureDelegations(file)
	if g.delegations.projectsErr == nil {
		t.Fatal("setup failure not observed")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	root, err = g.delegations.openProjects(".")
	if err != nil || root == nil {
		t.Fatal("old setup error blocked repaired tree", err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestContextWriterDoesNotCreateAnEscapingMissingTree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	g := &Gateway{delegations: &delegations{projects: path}}
	state := &contextState{journal: "../escape.clauduct-context.json", identity: "s|"}
	state.route.Model = "gpt-5.6-sol"
	if !errors.Is(g.saveContext(state), errContextJournal) {
		t.Fatal("escaping save accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid write created a projects tree")
	}
}
