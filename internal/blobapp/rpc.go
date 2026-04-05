package blobapp

import (
	"fmt"
	"path/filepath"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	blobcore "azure-storage/internal/blobapp/core"
	blobrpc "azure-storage/internal/blobapp/rpc"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

type rpcSessionCreatedMsg struct {
	snapshot blobcore.Snapshot
	err      error
}

type rpcStateLoadedMsg struct {
	snapshot blobcore.Snapshot
	err      error
}

type rpcActionLoadedMsg struct {
	result blobrpc.ActionInvokeResult
	err    error
}

func (m Model) rpcEnabled() bool {
	return m.client != nil
}

func rpcCreateSessionCmd(client *blobrpc.Client) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := client.CreateSession()
		return rpcSessionCreatedMsg{snapshot: snapshot, err: err}
	}
}

func rpcStateCmd(client *blobrpc.Client) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := client.GetState()
		return rpcStateLoadedMsg{snapshot: snapshot, err: err}
	}
}

func rpcActionCmd(client *blobrpc.Client, req blobcore.ActionRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := client.InvokeAction(req)
		return rpcActionLoadedMsg{result: result, err: err}
	}
}

func (m *Model) applySnapshot(snapshot blobcore.Snapshot) {
	m.loading = snapshot.Loading
	m.status = snapshot.Status
	m.lastErr = snapshot.LastErr
	m.hasSubscription = snapshot.HasSubscription
	if snapshot.CurrentSubscription != nil {
		m.currentSub = azure.Subscription{ID: snapshot.CurrentSubscription.ID, Name: snapshot.CurrentSubscription.Name, State: snapshot.CurrentSubscription.State}
	} else {
		m.currentSub = azure.Subscription{}
	}
	m.hasAccount = snapshot.HasAccount
	if snapshot.CurrentAccount != nil {
		m.currentAccount = blob.Account{
			Name:           snapshot.CurrentAccount.Name,
			SubscriptionID: snapshot.CurrentAccount.SubscriptionID,
			ResourceGroup:  snapshot.CurrentAccount.ResourceGroup,
			BlobEndpoint:   snapshot.CurrentAccount.BlobEndpoint,
		}
	} else {
		m.currentAccount = blob.Account{}
	}
	m.hasContainer = snapshot.HasContainer
	m.containerName = snapshot.ContainerName
	m.prefix = snapshot.Prefix
	m.blobLoadAll = snapshot.BlobLoadAll
	m.blobSearchQuery = snapshot.BlobSearchQuery
	m.visualLineMode = snapshot.VisualLineMode
	m.visualAnchor = snapshot.VisualAnchor
	m.focus = focusFromSnapshot(snapshot.Focus)

	m.subscriptions = nil
	for _, sub := range snapshot.Subscriptions {
		m.subscriptions = append(m.subscriptions, azure.Subscription{ID: sub.ID, Name: sub.Name, State: sub.State})
	}
	m.accounts = nil
	for _, account := range snapshot.Accounts {
		m.accounts = append(m.accounts, blob.Account{Name: account.Name, SubscriptionID: account.SubscriptionID, ResourceGroup: account.ResourceGroup, BlobEndpoint: account.BlobEndpoint})
	}
	m.containers = nil
	for _, container := range snapshot.Containers {
		m.containers = append(m.containers, blob.ContainerInfo{Name: container.Name})
	}
	m.blobs = nil
	byName := make(map[string]blob.BlobEntry, len(snapshot.Blobs))
	for _, entry := range snapshot.Blobs {
		blobEntry := blob.BlobEntry{Name: entry.Name, IsPrefix: entry.IsPrefix, Size: entry.Size, ContentType: entry.ContentType, AccessTier: entry.AccessTier}
		m.blobs = append(m.blobs, blobEntry)
		byName[entry.Name] = blobEntry
	}
	m.markedBlobs = make(map[string]blob.BlobEntry, len(snapshot.MarkedBlobNames))
	for _, name := range snapshot.MarkedBlobNames {
		if entry, ok := byName[name]; ok {
			m.markedBlobs[name] = entry
		}
	}

	ui.SetItemsPreserveIndex(&m.accountsList, accountsToItems(m.accounts))
	ui.SetItemsPreserveIndex(&m.containersList, containersToItems(m.containers))
	m.refreshBlobItems()

	if snapshot.Preview.Open {
		m.preview.open = true
		m.preview.blobName = snapshot.Preview.BlobName
		m.preview.blobSize = snapshot.Preview.BlobSize
		m.preview.contentType = snapshot.Preview.ContentType
		m.preview.binary = snapshot.Preview.Binary
		m.preview.cursor = snapshot.Preview.Cursor
		m.preview.windowStart = snapshot.Preview.WindowStart
		m.preview.windowData = []byte(snapshot.Preview.WindowText)
		m.preview.lineStarts = computeLineStarts(m.preview.windowData)
		m.preview.requestID = snapshot.Preview.RequestID
		m.preview.rendered = renderPreviewContent(m.preview.windowData, m.preview.blobName, m.preview.contentType, m.preview.binary, m.styles)
		m.preview.viewport.SetContent(m.preview.rendered)
		m.preview.viewport.YOffset = m.previewLocalLine()
	} else {
		m.resetPreviewState()
	}
	m.resize()
}

func focusFromSnapshot(focus string) int {
	switch focus {
	case "containers":
		return containersPane
	case "blobs":
		return blobsPane
	case "preview":
		return previewPane
	default:
		return accountsPane
	}
}

func (m Model) rpcRefresh() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionRefresh, HierarchyLimit: defaultHierarchyBlobLoadLimit, PrefixSearchLimit: defaultBlobPrefixSearchLimit, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcSelectSubscription(sub azure.Subscription) tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionSelectSubscription, Subscription: sub, HierarchyLimit: defaultHierarchyBlobLoadLimit, PrefixSearchLimit: defaultBlobPrefixSearchLimit, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcSelectAccount(account blob.Account) tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionSelectAccount, Account: account, HierarchyLimit: defaultHierarchyBlobLoadLimit, PrefixSearchLimit: defaultBlobPrefixSearchLimit, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcSelectContainer(name string) tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionSelectContainer, ContainerName: name, HierarchyLimit: defaultHierarchyBlobLoadLimit, PrefixSearchLimit: defaultBlobPrefixSearchLimit, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcOpenSelectedBlob(item blobItem) tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionOpenBlob, Blob: item.blob, HierarchyLimit: defaultHierarchyBlobLoadLimit, PrefixSearchLimit: defaultBlobPrefixSearchLimit, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcToggleLoadAll() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionToggleLoadAll, HierarchyLimit: defaultHierarchyBlobLoadLimit, PrefixSearchLimit: defaultBlobPrefixSearchLimit, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcNavigateLeft() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionNavigateLeft, HierarchyLimit: defaultHierarchyBlobLoadLimit, PrefixSearchLimit: defaultBlobPrefixSearchLimit, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcToggleMark(item blobItem) tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionToggleMark, Blob: item.blob, DisplayName: item.displayName, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcToggleVisual() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionToggleVisual, CurrentBlobName: m.currentBlobName(), SelectionCount: len(m.visualSelectionBlobNames()), VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcExitVisual() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionExitVisual, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcDownloadSelection() tea.Cmd {
	blobNameSet := make(map[string]struct{})
	for _, name := range m.sortedMarkedBlobNames() {
		blobNameSet[name] = struct{}{}
	}
	for _, name := range m.visualSelectionBlobNames() {
		blobNameSet[name] = struct{}{}
	}
	blobNames := sortedBlobNameSet(blobNameSet)
	if len(blobNames) == 0 {
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || item.blob.IsPrefix {
			return nil
		}
		blobNames = []string{item.blob.Name}
	}
	destinationRoot := filepath.Join(defaultDownloadRoot, m.currentAccount.Name, m.containerName)
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionDownload, BlobNames: blobNames, DestinationRoot: destinationRoot, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcPreviewMove(delta int) tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionPreviewMoveLines, LineDelta: delta, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcPreviewTop() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionPreviewTop, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcPreviewBottom() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionPreviewBottom, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcClosePreview() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionPreviewClose, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcNextFocus() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionFocusNext, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m Model) rpcPreviousFocus() tea.Cmd {
	return rpcActionCmd(m.client, blobcore.ActionRequest{Action: blobcore.ActionFocusPrevious, VisibleLines: max(1, m.preview.viewport.Height)})
}

func (m *Model) handleRPCMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case rpcSessionCreatedMsg:
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "Failed to create rpc session"
			return m, nil, true
		}
		m.sessionID = m.client.Session()
		m.applySnapshot(msg.snapshot)
		if m.hasSubscription {
			return m, m.rpcSelectSubscription(m.currentSub), true
		}
		return m, m.rpcRefresh(), true
	case rpcStateLoadedMsg:
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "Failed to load rpc state"
			return m, nil, true
		}
		m.applySnapshot(msg.snapshot)
		return m, nil, true
	case rpcActionLoadedMsg:
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "RPC action failed"
			return m, nil, true
		}
		m.applySnapshot(msg.result.State)
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) ensureRPCDownloadSelection() (tea.Cmd, bool) {
	cmd := m.rpcDownloadSelection()
	if cmd == nil {
		return nil, false
	}
	return cmd, true
}

func (m Model) rpcStatus(msg string) (Model, tea.Cmd) {
	m.status = msg
	return m, nil
}

func (m Model) rpcDebugState() string {
	return fmt.Sprintf("rpc=%t session=%t", m.rpcEnabled(), m.sessionID != "")
}
