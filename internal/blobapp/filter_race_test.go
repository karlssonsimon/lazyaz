package blobapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

var filterRaceNames = []string{
	"stavausprod", "sthplexflowprod", "sthplhtgprod", "sthplivecoprod",
	"sthtgexternalprod", "sthtgonpremprod", "sthtgpowerbiprod", "sthtgprod",
	"sthtgprodarc", "sthtgprodfordhmcfi", "sthtgprodfordhmcse", "sthtgprodhcm",
	"sthtgprodhpl", "sthtgprodiwastro", "sthtgrpaexternalprod", "sthtgstateprod",
}

func filterRaceModel() Model {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.DismissSpinner(m.LoadingSpinnerID)
	m.ClearLoading()
	m.Width, m.Height = 90, 40
	m.focus = accountsPane
	accounts := make([]blob.Account, len(filterRaceNames))
	for i, n := range filterRaceNames {
		accounts[i] = blob.Account{Name: n}
	}
	m.accounts = accounts
	m.accountsList.SetItems(accountsToItems(accounts))
	m.resize()
	return m
}

// sendCollect sends a message through the app and executes returned
// commands, collecting FilterMatchesMsg instead of delivering them —
// mimicking bubbles' async filter results still being in flight.
func sendCollect(m Model, msg tea.Msg, pending *[]tea.Msg) Model {
	updated, cmd := m.Update(msg)
	m = updated.(Model)
	cmds := []tea.Cmd{cmd}
	for len(cmds) > 0 {
		c := cmds[0]
		cmds = cmds[1:]
		if c == nil {
			continue
		}
		out := c()
		switch v := out.(type) {
		case tea.BatchMsg:
			for _, bc := range v {
				cmds = append(cmds, tea.Cmd(bc))
			}
		case list.FilterMatchesMsg:
			if pending != nil {
				*pending = append(*pending, v)
			}
		}
	}
	return m
}

// TestPaneFilterIsSynchronous types an exact-match query and expects
// the visible set to be correct immediately, without waiting for any
// async filter results.
func TestPaneFilterIsSynchronous(t *testing.T) {
	m := filterRaceModel()
	m = sendCollect(m, tea.KeyPressMsg{Code: '/', Text: "/"}, nil)
	for _, r := range "'htg" {
		m = sendCollect(m, tea.KeyPressMsg{Code: r, Text: string(r)}, nil)
	}
	if got := len(m.accountsList.VisibleItems()); got != 13 {
		t.Fatalf("visible = %d for query %q, want 13", got, m.accountsList.FilterValue())
	}
}

// TestAccountFilterMatchesVisibleTextOnly pins that filtering does not
// match hidden metadata: an account whose resource group contains the
// query but whose name doesn't must stay hidden — otherwise rows show
// up with no visible reason and no match highlight.
func TestAccountFilterMatchesVisibleTextOnly(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.DismissSpinner(m.LoadingSpinnerID)
	m.ClearLoading()
	m.Width, m.Height = 90, 40
	m.focus = accountsPane
	m.accounts = []blob.Account{
		{Name: "stavausprod", ResourceGroup: "rg-htg-prod", SubscriptionID: "sub-1"},
		{Name: "sthtgprod", ResourceGroup: "rg-other", SubscriptionID: "sub-1"},
	}
	m.accountsList.SetItems(accountsToItems(m.accounts))
	m.resize()

	m = sendCollect(m, tea.KeyPressMsg{Code: '/', Text: "/"}, nil)
	for _, r := range "'htg" {
		m = sendCollect(m, tea.KeyPressMsg{Code: r, Text: string(r)}, nil)
	}

	visible := m.accountsList.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("visible = %d for query 'htg, want 1 (name matches only)", len(visible))
	}
	if got := visible[0].(accountItem).account.Name; got != "sthtgprod" {
		t.Fatalf("visible account = %q, want sthtgprod", got)
	}
}

// TestPaneFilterStaleAsyncResultIgnored replays the race that showed
// the full unfiltered list under an exact query: bubbles runs each
// keystroke's filterItems in its own goroutine and applies results
// last-writer-wins, so the quote-only prefix result (a match-all under
// fzf syntax) could arrive after the final one and stick. The app now
// filters synchronously and drops FilterMatchesMsg, so even an
// adversarial delivery order must not disturb the visible set.
func TestPaneFilterStaleAsyncResultIgnored(t *testing.T) {
	m := filterRaceModel()

	var pending []tea.Msg
	m = sendCollect(m, tea.KeyPressMsg{Code: '/', Text: "/"}, &pending)
	for _, r := range "'htg" {
		m = sendCollect(m, tea.KeyPressMsg{Code: r, Text: string(r)}, &pending)
	}
	if len(pending) == 0 {
		t.Fatal("expected in-flight async filter results to exist")
	}

	// Deliver newest first so the stale quote-only result lands last.
	for i := len(pending) - 1; i >= 0; i-- {
		updated, _ := m.Update(pending[i])
		m = updated.(Model)
	}

	if got := len(m.accountsList.VisibleItems()); got != 13 {
		t.Fatalf("stale filter result disturbed the view: visible = %d for query %q, want 13",
			got, m.accountsList.FilterValue())
	}
}
