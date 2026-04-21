package sbapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func newStyles() ui.Styles {
	return ui.NewStyles(testConfig.ActiveScheme())
}

func newEntityList(width int, items []list.Item) list.Model {
	styles := newStyles()
	l := list.New(items, newEntityDelegate(styles.Delegate, styles), width, 10)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	return l
}

func newSubList(width int, items []list.Item) list.Model {
	styles := newStyles()
	l := list.New(items, newSubscriptionDelegate(styles.Delegate, styles), width, 10)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	return l
}

func TestEntityDelegate_RendersCounts(t *testing.T) {
	items := []list.Item{
		entityItem{entity: servicebus.Entity{Name: "orders", Kind: servicebus.EntityQueue, ActiveMsgCount: 12, DeadLetterCount: 0}},
		entityItem{entity: servicebus.Entity{Name: "payments", Kind: servicebus.EntityQueue, ActiveMsgCount: 0, DeadLetterCount: 3}},
		entityItem{entity: servicebus.Entity{Name: "events", Kind: servicebus.EntityTopic, ActiveMsgCount: 128, DeadLetterCount: 5}},
	}
	l := newEntityList(60, items)

	plain := ansi.Strip(l.View())
	want := []string{"orders", "12 / 0", "payments", "0 / 3", "events", "128 / 5"}
	for _, w := range want {
		if !strings.Contains(plain, w) {
			t.Errorf("expected %q in rendered output, got:\n%s", w, plain)
		}
	}
}

func TestEntityDelegate_DangerStyleWhenDLQ(t *testing.T) {
	styles := newStyles()
	dangerStyled := lipgloss.NewStyle().Foreground(styles.Danger.GetForeground()).Render("0 / 3")

	items := []list.Item{
		entityItem{entity: servicebus.Entity{Name: "q", Kind: servicebus.EntityQueue, ActiveMsgCount: 0, DeadLetterCount: 3}},
	}
	// Not the selected row: selection is at index 0 by default, so give
	// it a different index-0 item to ensure DLQ row isn't selected.
	items = append([]list.Item{
		entityItem{entity: servicebus.Entity{Name: "first", Kind: servicebus.EntityQueue}},
	}, items...)
	l := newEntityList(60, items)

	raw := l.View()
	if !strings.Contains(raw, dangerStyled) {
		t.Errorf("expected DLQ count to be rendered in danger color\nraw:\n%s\nwanted substring:\n%q", raw, dangerStyled)
	}
}

func TestEntityDelegate_NoCountsWhenDLQZero_NotDangerStyled(t *testing.T) {
	styles := newStyles()
	dangerStyled := lipgloss.NewStyle().Foreground(styles.Danger.GetForeground()).Render("12 / 0")

	items := []list.Item{
		entityItem{entity: servicebus.Entity{Name: "q", Kind: servicebus.EntityQueue, ActiveMsgCount: 12, DeadLetterCount: 0}},
	}
	l := newEntityList(60, items)

	raw := l.View()
	if strings.Contains(raw, dangerStyled) {
		t.Errorf("DLQ=0 count should not be styled in danger color\nraw: %s", raw)
	}
}

func TestEntityDelegate_NarrowWidthDropsCounts(t *testing.T) {
	items := []list.Item{
		entityItem{entity: servicebus.Entity{Name: "a-longish-queue-name", Kind: servicebus.EntityQueue, ActiveMsgCount: 999, DeadLetterCount: 999}},
	}
	l := newEntityList(20, items)

	plain := ansi.Strip(l.View())
	// The list itself may ellipsis-truncate the name, which is expected;
	// our delegate's responsibility is only to omit counts at narrow width.
	if !strings.Contains(plain, "a-longish") {
		t.Errorf("name prefix missing, got: %s", plain)
	}
	if strings.Contains(plain, "999 / 999") {
		t.Errorf("narrow pane should drop counts, got: %s", plain)
	}
}

func TestEntityDelegate_CountsAlignNearNames(t *testing.T) {
	// Widest title (glyph + space + name) is "≡ payments" = 10 runes.
	// After base left-pad (2) and countsGap (4), counts should start
	// at column 16 — well before the right edge of a 60-wide list.
	items := []list.Item{
		entityItem{entity: servicebus.Entity{Name: "orders", Kind: servicebus.EntityQueue, ActiveMsgCount: 1, DeadLetterCount: 0}},
		entityItem{entity: servicebus.Entity{Name: "payments", Kind: servicebus.EntityQueue, ActiveMsgCount: 2, DeadLetterCount: 0}},
	}
	l := newEntityList(60, items)

	plain := ansi.Strip(l.View())
	for _, line := range strings.Split(plain, "\n") {
		col := strings.Index(line, "1 / 0")
		if col < 0 {
			continue
		}
		if col > 24 {
			t.Errorf("counts column %d too far right; names are short and list is 60 wide\nline: %q", col, line)
		}
		return
	}
	t.Fatalf("no line contained counts:\n%s", plain)
}

func TestSubscriptionDelegate_RendersCounts(t *testing.T) {
	items := []list.Item{
		subscriptionItem{sub: servicebus.TopicSubscription{Name: "orders-sub", ActiveMsgCount: 7, DeadLetterCount: 2}},
	}
	l := newSubList(60, items)

	plain := ansi.Strip(l.View())
	if !strings.Contains(plain, "orders-sub") || !strings.Contains(plain, "7 / 2") {
		t.Errorf("subscription row missing name or counts: %s", plain)
	}
}
