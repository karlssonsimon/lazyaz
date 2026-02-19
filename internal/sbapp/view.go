package sbapp

import (
	"fmt"
	"strings"

	commonui "azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	styles := commonui.NewChromeStyles(uiPalette(m.ui))

	subscriptionName := "-"
	namespaceName := "-"
	entityName := "-"
	if m.hasSubscription {
		subscriptionName = subscriptionDisplayName(m.currentSub)
	}
	if m.hasNamespace {
		namespaceName = m.currentNS.Name
	}
	if m.hasEntity {
		entityName = entityDisplayName(m.currentEntity)
	}

	header := styles.Header.Width(m.width).Render(commonui.TrimToWidth("Azure Service Bus Explorer", m.width-2))
	headerMeta := styles.Meta.Width(m.width).Render(commonui.TrimToWidth(fmt.Sprintf("Subscription: %s | Namespace: %s | Entity: %s", subscriptionName, namespaceName, entityName), m.width-2))

	m.subscriptionsList.Title = m.subscriptionsPaneTitle()
	m.namespacesList.Title = m.namespacesPaneTitle()
	m.entitiesList.Title = m.entitiesPaneTitle()
	m.detailList.Title = m.detailPaneTitle()

	if m.deadLetter && m.detailMode == detailMessages {
		m.detailList.Styles.Title = m.detailList.Styles.Title.
			Foreground(lipgloss.Color(m.ui.Danger))
	} else {
		m.detailList.Styles.Title = m.detailList.Styles.Title.
			Foreground(lipgloss.Color(m.ui.Accent))
	}

	subscriptionsView := m.subscriptionsList.View()
	namespacesView := m.namespacesList.View()
	entitiesView := m.entitiesList.View()
	detailView := m.detailList.View()

	subscriptionsPaneStyle := styles.Pane.Copy().MarginRight(1)
	namespacesPaneStyle := styles.Pane.Copy().MarginRight(1)
	entitiesPaneStyle := styles.Pane.Copy().MarginRight(1)
	detailPaneStyle := styles.Pane.Copy()

	if m.focus == subscriptionsPane {
		subscriptionsPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}
	if m.focus == namespacesPane {
		namespacesPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}
	if m.focus == entitiesPane {
		entitiesPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}

	if m.deadLetter && m.detailMode == detailMessages {
		detailPaneStyle = styles.Pane.Copy().BorderForeground(lipgloss.Color(m.ui.Danger))
	} else if m.focus == detailPane && !m.viewingMessage {
		detailPaneStyle = styles.FocusedPane.Copy()
	}
	if m.viewingMessage {
		detailPaneStyle = detailPaneStyle.Copy().MarginRight(1)
	}

	panesList := []string{
		subscriptionsPaneStyle.Render(subscriptionsView),
		namespacesPaneStyle.Render(namespacesView),
		entitiesPaneStyle.Render(entitiesView),
		detailPaneStyle.Render(detailView),
	}

	if m.viewingMessage {
		previewTitleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(m.ui.Accent)).
			Padding(0, 1)
		msgID := commonui.EmptyToDash(m.selectedMessage.MessageID)
		previewTitle := previewTitleStyle.Render(fmt.Sprintf("Message: %s", msgID))
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, m.messageViewport.View())

		previewPaneStyle := styles.FocusedPane.Copy()
		panesList = append(panesList, previewPaneStyle.Render(previewContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, panesList...)

	filterHint := "Press / to filter the focused pane (fzf-style live filter)."
	if m.focusedListSettingFilter() {
		filterHint = fmt.Sprintf("Filtering %s: type to narrow, up/down to move, Enter applies filter.", paneName(m.focus))
	}
	filterLine := styles.FilterHint.Width(m.width).Render(commonui.TrimToWidth(filterHint, m.width-2))

	errorLine := ""
	if m.lastErr != "" {
		errorLine = styles.Error.Width(m.width).Render(commonui.TrimToWidth("Error: "+m.lastErr, m.width-2))
	}

	statusText := m.status
	if m.loading {
		statusText = fmt.Sprintf("%s %s", m.spinner.View(), m.status)
	}
	statusLine := styles.Status.Width(m.width).Render(commonui.TrimToWidth(statusText, m.width-2))

	helpLine := styles.Help.Width(m.width).Render(commonui.TrimToWidth(m.keymap.HelpText(), m.width-2))

	parts := []string{header, headerMeta, panes, filterLine}
	if errorLine != "" {
		parts = append(parts, errorLine)
	}
	parts = append(parts, statusLine, helpLine)

	view := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if m.selectingTheme {
		view = m.overlayThemeSelector(view)
	}

	return view
}

func (m Model) overlayThemeSelector(base string) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.ui.Accent)).
		Padding(0, 1)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.ui.Text)).
		Padding(0, 1)

	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.ui.SelectedText)).
		Background(lipgloss.Color(m.ui.SelectedBg)).
		Bold(true).
		Padding(0, 1)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.ui.Muted)).
		Padding(0, 1)

	var rows []string
	rows = append(rows, titleStyle.Render("Select Theme"))
	rows = append(rows, "")

	maxNameLen := 0
	for _, t := range m.themes {
		if len(t.Name) > maxNameLen {
			maxNameLen = len(t.Name)
		}
	}
	_ = maxNameLen

	for i, t := range m.themes {
		marker := "  "
		if i == m.activeThemeIdx {
			marker = "* "
		}
		label := marker + t.Name
		if i == m.themeIdx {
			rows = append(rows, cursorStyle.Render(label))
		} else {
			rows = append(rows, normalStyle.Render(label))
		}
	}

	rows = append(rows, "")
	rows = append(rows, hintStyle.Render("j/k navigate | enter apply | esc cancel"))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.ui.BorderFocused)).
		Padding(1, 2)

	box := boxStyle.Render(content)

	return placeOverlay(m.width, m.height, box, base)
}

func placeOverlay(width, height int, overlay, base string) string {
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	oH := len(overlayLines)
	oW := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > oW {
			oW = w
		}
	}

	startY := (height - oH) / 2
	startX := (width - oW) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, ol := range overlayLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		line := baseLines[row]
		lineW := lipgloss.Width(line)

		var out strings.Builder
		if startX > 0 {
			if lineW >= startX {
				out.WriteString(truncateAnsi(line, startX))
			} else {
				out.WriteString(line)
				out.WriteString(strings.Repeat(" ", startX-lineW))
			}
		}
		out.WriteString(ol)
		baseLines[row] = out.String()
	}

	return strings.Join(baseLines[:height], "\n")
}

func truncateAnsi(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		candidate := string(runes[:i])
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return ""
}
