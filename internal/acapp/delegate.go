package acapp

import (
	"fmt"
	"io"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	// suffixGap is the minimum space between the widest key and the
	// label column.
	suffixGap = 3
	// minKeyWidth is the least a cropped key keeps before the suffix is
	// dropped instead.
	minKeyWidth = 12
)

// settingDelegate renders a key-value row as the key with the label
// (and a kind tag for references and flags) aligned in a muted column
// to the right, cropping long keys so the column stays visible.
type settingDelegate struct {
	base     list.DefaultDelegate
	normal   lipgloss.Style
	selected lipgloss.Style
}

func newSettingDelegate(base list.DefaultDelegate, styles ui.Styles) settingDelegate {
	sel := lipgloss.NewStyle().
		Foreground(base.Styles.SelectedTitle.GetForeground()).
		Background(base.Styles.SelectedTitle.GetBackground())
	return settingDelegate{
		base:     base,
		normal:   lipgloss.NewStyle().Foreground(styles.Muted.GetForeground()),
		selected: sel,
	}
}

func (d settingDelegate) Height() int  { return d.base.Height() }
func (d settingDelegate) Spacing() int { return d.base.Spacing() }
func (d settingDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.base.Update(msg, m)
}

// rowSuffix is the muted text after a key: its label, then a tag for
// Key Vault references and feature flags.
func rowSuffix(s appconfig.Setting) string {
	var parts []string
	if s.Label != "" {
		parts = append(parts, s.Label)
	}
	switch {
	case s.IsKeyVaultReference():
		parts = append(parts, "→ key vault")
	case s.IsFeatureFlag():
		parts = append(parts, "flag")
	}
	return strings.Join(parts, " · ")
}

func (d settingDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(settingItem)
	if !ok {
		d.base.Render(w, m, index, item)
		return
	}
	suffix := rowSuffix(si.setting)
	if suffix == "" {
		d.base.Render(w, m, index, item)
		return
	}

	rowWidth := m.Width()
	suffixW := lipgloss.Width(suffix)
	colW := max(suffixColumnWidth(m), suffixW)

	targetCol := titleColumnWidth(m) + suffixGap
	if rowWidth > 0 && targetCol+colW > rowWidth {
		avail := rowWidth - suffixGap - colW
		if avail < minKeyWidth {
			d.base.Render(w, m, index, item)
			return
		}
		m.SetWidth(avail) // m is a copy; narrows the title truncation
		targetCol = rowWidth - colW
	}

	var buf strings.Builder
	d.base.Render(&buf, m, index, item)
	rendered := buf.String()
	renderedW := lipgloss.Width(rendered)
	if targetCol < renderedW+1 {
		targetCol = renderedW + 1
	}
	if rowWidth <= 0 || targetCol+suffixW > rowWidth {
		fmt.Fprint(w, rendered)
		return
	}
	style := d.normal
	if index == m.Index() && m.FilterState() != list.Filtering {
		style = d.selected
	}
	fmt.Fprint(w, rendered+strings.Repeat(" ", targetCol-renderedW)+style.Render(suffix))
}

func suffixColumnWidth(m list.Model) int {
	widest := 0
	for _, it := range m.VisibleItems() {
		if si, ok := it.(settingItem); ok {
			if w := lipgloss.Width(rowSuffix(si.setting)); w > widest {
				widest = w
			}
		}
	}
	return widest
}

// titleColumnWidth is the rendered width of the widest visible title
// plus the base delegate's left padding.
func titleColumnWidth(m list.Model) int {
	widest := 0
	for _, it := range m.VisibleItems() {
		if di, ok := it.(list.DefaultItem); ok {
			if w := lipgloss.Width(di.Title()); w > widest {
				widest = w
			}
		}
	}
	return widest + 2
}
