package sbapp

import (
	"fmt"
	"time"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

// withPaneSpinner is a thin wrapper around ui.RenderPaneSpinner that
// checks whether the given pane is the current loading target.
func (m Model) withPaneSpinner(title string, pane int, width int) string {
	loading := m.Loading && m.LoadingPane == pane
	return ui.RenderPaneSpinner(title, loading, m.LoadingStartedAt, m.Styles, width)
}

func (m Model) View() string {
	if m.Width == 0 || m.Height == 0 {
		return "loading..."
	}

	styles := m.Styles.Chrome

	var sbItems []ui.StatusBarItem
	if m.hasNamespace {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Namespace:", Value: m.currentNS.Name})
	}
	if m.hasEntity {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Entity:", Value: entityDisplayName(m.currentEntity)})
	}

	pw := m.paneWidths
	pane := m.Styles.Chrome.Pane
	m.namespacesList.Title = m.withPaneSpinner(m.namespacesPaneTitle(), namespacesPane, ui.PaneContentWidth(pane, pw[0]))
	m.entitiesList.Title = m.withPaneSpinner(m.entitiesPaneTitle(), entitiesPane, ui.PaneContentWidth(pane, pw[1]))
	m.detailList.Title = m.withPaneSpinner(m.detailPaneTitle(), detailPane, ui.PaneContentWidth(pane, pw[2]))

	if m.deadLetter && m.detailMode == detailMessages {
		m.detailList.Styles.Title = m.Styles.DangerBold.Padding(0, 1)
	} else {
		m.detailList.Styles.Title = m.Styles.List.Title
	}

	ui.ClampListSelection(&m.namespacesList)
	ui.ClampListSelection(&m.entitiesList)
	ui.ClampListSelection(&m.detailList)

	km := m.Keymap

	nsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.FilterInput.Short(), "filter"},
		{km.NextFocus.Short(), "next"},
		{km.SubscriptionPicker.Short(), "sub"},
		{km.Inspect.Short(), "inspect"},
	}, m.Styles, ui.PaneContentWidth(pane, pw[0]))

	entHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.NavigateLeft.Short(), "back"},
		{km.ToggleDLQFilter.Short(), "DLQ filter"},
	}, m.Styles, ui.PaneContentWidth(pane, pw[1]))

	detHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.ToggleMark.Short(), "mark"},
		{km.ShowActiveQueue.Short() + "/" + km.ShowDeadLetterQueue.Short(), "active/DLQ"},
		{km.RequeueDLQ.Short(), "requeue"},
	}, m.Styles, ui.PaneContentWidth(pane, pw[2]))

	namespacesView := lipgloss.JoinVertical(lipgloss.Left, m.namespacesList.View(), nsHints)
	entitiesView := lipgloss.JoinVertical(lipgloss.Left, m.entitiesList.View(), entHints)
	detailView := lipgloss.JoinVertical(lipgloss.Left, m.detailList.View(), detHints)

	h := m.paneHeight
	namespacesPaneStyle := styles.Pane.Copy().Width(pw[0]).Height(h)
	entitiesPaneStyle := styles.Pane.Copy().Width(pw[1]).Height(h)
	detailPaneStyle := styles.Pane.Copy().Width(pw[2]).Height(h)

	if m.focus == namespacesPane {
		namespacesPaneStyle = styles.FocusedPane.Copy().Width(pw[0]).Height(h)
	}
	if m.focus == entitiesPane {
		entitiesPaneStyle = styles.FocusedPane.Copy().Width(pw[1]).Height(h)
	}

	if m.deadLetter && m.detailMode == detailMessages {
		detailPaneStyle = styles.Pane.Copy().Width(pw[2]).Height(h).BorderForeground(m.Styles.Danger.GetForeground())
	} else if m.focus == detailPane && !m.viewingMessage {
		detailPaneStyle = styles.FocusedPane.Copy().Width(pw[2]).Height(h)
	}

	panesList := []string{
		namespacesPaneStyle.Render(namespacesView),
		entitiesPaneStyle.Render(entitiesView),
		detailPaneStyle.Render(detailView),
	}

	if m.viewingMessage {
		previewTitleStyle := m.Styles.Accent.Copy().Padding(0, 1)
		msgID := ui.EmptyToDash(m.selectedMessage.MessageID)
		previewTitle := previewTitleStyle.Render(fmt.Sprintf("Message: %s", msgID))
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, m.messageViewport.View())

		previewPaneStyle := styles.FocusedPane.Copy().Width(pw[3]).Height(h)
		panesList = append(panesList, previewPaneStyle.Render(previewContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, panesList...)

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)

	sbStatus := m.Status
	sbErr := m.LastErr != ""
	if sbErr {
		sbStatus = m.LastErr
	} else if m.Loading {
		sbStatus = ui.SpinnerFrameAt(time.Since(m.LoadingStartedAt)) + " " + m.Status
	}
	statusBar := ui.RenderStatusBar(m.Styles, sbItems, sbStatus, sbErr, m.Width)

	parts := []string{subBar, panes, statusBar}

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, parts...), m.Width, m.Height, m.Styles.Bg)
	return m.RenderOverlays(view)
}

