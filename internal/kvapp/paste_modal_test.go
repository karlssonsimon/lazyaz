package kvapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

func TestParsePasteBundleAcceptsObject(t *testing.T) {
	in := `{"db-password": "hunter2", "api-key": "sk-..."}`
	got, err := parsePasteBundle(in)
	if err != nil {
		t.Fatalf("parsePasteBundle err = %v", err)
	}
	if len(got) != 2 || got["db-password"] != "hunter2" || got["api-key"] != "sk-..." {
		t.Fatalf("parsePasteBundle = %v, want db-password+api-key map", got)
	}
}

func TestParsePasteBundleRejectsBadShape(t *testing.T) {
	cases := map[string]string{
		"empty":           ``,
		"whitespace":      `   `,
		"not json":        `hunter2`,
		"array":           `["a", "b"]`,
		"scalar":          `"hunter2"`,
		"nested object":   `{"db-password": {"value": "hunter2"}}`,
		"number value":    `{"db-password": 42}`,
		"null value":      `{"db-password": null}`,
		"empty object":    `{}`,
		"bad name char":   `{"db pwd": "hunter2"}`,
		"bad name length": `{"` + strings.Repeat("a", 128) + `": "hunter2"}`,
		"empty name":      `{"": "hunter2"}`,
	}
	for label, in := range cases {
		t.Run(label, func(t *testing.T) {
			if _, err := parsePasteBundle(in); err == nil {
				t.Fatalf("parsePasteBundle(%q) returned nil err, want rejection", in)
			}
		})
	}
}

func TestNewPasteModalDefaultsMatchExisting(t *testing.T) {
	existing := []keyvault.Secret{{Name: "db-password"}, {Name: "smtp-token"}}
	state, err := newPasteModalState(
		`{"db-password": "hunter2", "api-key": "sk-..."}`,
		existing,
	)
	if err != nil {
		t.Fatalf("newPasteModalState err = %v", err)
	}
	if !state.active {
		t.Fatal("expected modal active")
	}
	if len(state.rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(state.rows))
	}

	byName := map[string]pasteRow{}
	for _, r := range state.rows {
		byName[r.name] = r
	}
	if r := byName["db-password"]; !r.existsInVault || r.skip || r.value != "hunter2" {
		t.Fatalf("db-password row = %+v, want existsInVault=true skip=false value=hunter2", r)
	}
	if r := byName["api-key"]; r.existsInVault || r.skip || r.value != "sk-..." {
		t.Fatalf("api-key row = %+v, want existsInVault=false skip=false value=sk-...", r)
	}
}

func TestPasteModalRowsSortedByName(t *testing.T) {
	state, err := newPasteModalState(
		`{"zeta": "1", "alpha": "2", "mike": "3"}`,
		nil,
	)
	if err != nil {
		t.Fatalf("newPasteModalState err = %v", err)
	}
	want := []string{"alpha", "mike", "zeta"}
	for i, w := range want {
		if state.rows[i].name != w {
			t.Fatalf("rows[%d] = %q, want %q", i, state.rows[i].name, w)
		}
	}
}

func TestPasteModalToggleSkip(t *testing.T) {
	state, _ := newPasteModalState(`{"a": "1", "b": "2"}`, nil)
	km := keymap.Default()

	// Cursor on row 0 ("a"). Toggle: skip.
	state = state.toggleCurrentSkip(km)
	if !state.rows[0].skip || state.rows[1].skip {
		t.Fatalf("after toggle row0: rows = %+v, want only row 0 skipped", state.rows)
	}
	// Toggle again: include.
	state = state.toggleCurrentSkip(km)
	if state.rows[0].skip {
		t.Fatal("second toggle should unskip row 0")
	}
}

func TestPasteModalVisualLineBulkSkip(t *testing.T) {
	state, _ := newPasteModalState(`{"a": "1", "b": "2", "c": "3"}`, nil)

	// Anchor at row 0, cursor at row 2, then toggle.
	state.cursor = 0
	state = state.toggleVisual()
	if !state.visual {
		t.Fatal("expected visual mode on")
	}
	state.cursor = 2
	state = state.toggleCurrentSkipVisual()
	if !state.rows[0].skip || !state.rows[1].skip || !state.rows[2].skip {
		t.Fatalf("visual range toggle should skip all three: %+v", state.rows)
	}
	if state.visual {
		t.Fatal("expected visual mode off after range action")
	}
}

func TestPasteModalPlanCounts(t *testing.T) {
	existing := []keyvault.Secret{{Name: "a"}, {Name: "b"}}
	state, _ := newPasteModalState(`{"a": "1", "b": "2", "c": "3", "d": "4"}`, existing)
	state.rows[3].skip = true // skip "d"

	plan := state.Plan()
	if plan.NewVersion != 2 || plan.Create != 1 || plan.Skip != 1 {
		t.Fatalf("Plan = %+v, want newVersion=2 create=1 skip=1", plan)
	}
	if len(plan.Apply) != 3 {
		t.Fatalf("Plan.Apply has %d rows, want 3 (a, b, c)", len(plan.Apply))
	}
}
