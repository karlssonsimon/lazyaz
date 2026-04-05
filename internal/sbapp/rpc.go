package sbapp

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	sbcore "azure-storage/internal/sbapp/core"
	sbrpc "azure-storage/internal/sbapp/rpc"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

type rpcSessionCreatedMsg struct {
	snapshot sbcore.Snapshot
	err      error
}

type rpcStateLoadedMsg struct {
	snapshot sbcore.Snapshot
	err      error
}

type rpcActionLoadedMsg struct {
	result sbrpc.ActionInvokeResult
	err    error
}

func (m Model) rpcEnabled() bool { return m.client != nil }

func rpcCreateSessionCmd(client *sbrpc.Client) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := client.CreateSession()
		return rpcSessionCreatedMsg{snapshot: snapshot, err: err}
	}
}

func rpcActionCmd(client *sbrpc.Client, req sbcore.ActionRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := client.InvokeAction(req)
		return rpcActionLoadedMsg{result: result, err: err}
	}
}

func (m *Model) applySnapshot(snapshot sbcore.Snapshot) {
	m.loading = snapshot.Loading
	m.status = snapshot.Status
	m.lastErr = snapshot.LastErr
	m.hasSubscription = snapshot.HasSubscription
	if snapshot.CurrentSubscription != nil {
		m.currentSub = azure.Subscription{ID: snapshot.CurrentSubscription.ID, Name: snapshot.CurrentSubscription.Name, State: snapshot.CurrentSubscription.State}
	} else {
		m.currentSub = azure.Subscription{}
	}
	m.hasNamespace = snapshot.HasNamespace
	if snapshot.CurrentNamespace != nil {
		m.currentNS = servicebus.Namespace{Name: snapshot.CurrentNamespace.Name, SubscriptionID: snapshot.CurrentNamespace.SubscriptionID, ResourceGroup: snapshot.CurrentNamespace.ResourceGroup, FQDN: snapshot.CurrentNamespace.FQDN}
	} else {
		m.currentNS = servicebus.Namespace{}
	}
	m.hasEntity = snapshot.HasEntity
	if snapshot.CurrentEntity != nil {
		m.currentEntity = servicebus.Entity{Name: snapshot.CurrentEntity.Name, Kind: snapshot.CurrentEntity.Kind, ActiveMsgCount: snapshot.CurrentEntity.ActiveMsgCount, DeadLetterCount: snapshot.CurrentEntity.DeadLetterCount}
	} else {
		m.currentEntity = servicebus.Entity{}
	}
	m.viewingTopicSub = snapshot.ViewingTopicSub
	if snapshot.CurrentTopicSub != nil {
		m.currentTopicSub = servicebus.TopicSubscription{Name: snapshot.CurrentTopicSub.Name}
	} else {
		m.currentTopicSub = servicebus.TopicSubscription{}
	}
	m.deadLetter = snapshot.DeadLetter
	m.dlqFilter = snapshot.DLQFilter
	m.viewingMessage = snapshot.ViewingMessage
	if snapshot.SelectedMessage != nil {
		m.selectedMessage = servicebus.PeekedMessage{MessageID: snapshot.SelectedMessage.MessageID, FullBody: snapshot.SelectedMessage.FullBody}
	} else {
		m.selectedMessage = servicebus.PeekedMessage{}
	}
	m.focus = int(sbcore.PaneFromName(snapshot.Focus))
	m.detailMode = detailView(sbcore.DetailModeFromName(snapshot.DetailMode))

	m.subscriptions = nil
	for _, sub := range snapshot.Subscriptions {
		m.subscriptions = append(m.subscriptions, azure.Subscription{ID: sub.ID, Name: sub.Name, State: sub.State})
	}
	m.namespaces = nil
	for _, ns := range snapshot.Namespaces {
		m.namespaces = append(m.namespaces, servicebus.Namespace{Name: ns.Name, SubscriptionID: ns.SubscriptionID, ResourceGroup: ns.ResourceGroup, FQDN: ns.FQDN})
	}
	m.entities = nil
	for _, entity := range snapshot.Entities {
		m.entities = append(m.entities, servicebus.Entity{Name: entity.Name, Kind: entity.Kind, ActiveMsgCount: entity.ActiveMsgCount, DeadLetterCount: entity.DeadLetterCount})
	}
	m.topicSubs = nil
	for _, sub := range snapshot.TopicSubs {
		m.topicSubs = append(m.topicSubs, servicebus.TopicSubscription{Name: sub.Name})
	}
	m.peekedMessages = nil
	for _, msg := range snapshot.PeekedMessages {
		m.peekedMessages = append(m.peekedMessages, servicebus.PeekedMessage{MessageID: msg.MessageID, FullBody: msg.FullBody, BodyPreview: msg.FullBody})
	}
	m.markedMessages = make(map[string]struct{}, len(snapshot.MarkedMessageIDs))
	for _, id := range snapshot.MarkedMessageIDs {
		m.markedMessages[id] = struct{}{}
	}
	m.duplicateMessages = make(map[string]struct{}, len(snapshot.DuplicateMessageIDs))
	for _, id := range snapshot.DuplicateMessageIDs {
		m.duplicateMessages[id] = struct{}{}
	}

	ui.SetItemsPreserveIndex(&m.namespacesList, namespacesToItems(m.namespaces))
	ui.SetItemsPreserveIndex(&m.entitiesList, entitiesToFilteredItems(m.entities, m.dlqFilter))
	if m.detailMode == detailTopicSubscriptions && !m.viewingTopicSub {
		ui.SetItemsPreserveIndex(&m.detailList, topicSubsToItems(m.topicSubs))
	} else {
		ui.SetItemsPreserveIndex(&m.detailList, messagesToItems(m.peekedMessages, m.markedMessages, m.duplicateMessages))
	}
	if m.viewingMessage {
		m.messageViewport.SetContent(m.styles.Syntax.HighlightJSON(m.selectedMessage.FullBody))
	}
	m.resize()
}

func (m Model) rpcRefresh() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionRefresh})
}
func (m Model) rpcFocusNext() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionFocusNext})
}
func (m Model) rpcFocusPrevious() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionFocusPrevious})
}
func (m Model) rpcNavigateLeft() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionNavigateLeft})
}
func (m Model) rpcBackspace() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionBackspace})
}
func (m Model) rpcSelectSubscription(sub azure.Subscription) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionSelectSubscription, Subscription: sub})
}
func (m Model) rpcSelectNamespace(ns servicebus.Namespace) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionSelectNamespace, Namespace: ns})
}
func (m Model) rpcSelectEntity(entity servicebus.Entity) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionSelectEntity, Entity: entity})
}
func (m Model) rpcSelectTopicSub(sub servicebus.TopicSubscription) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionSelectTopicSub, TopicSub: sub})
}
func (m Model) rpcOpenMessage(msg servicebus.PeekedMessage) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionOpenMessage, Message: msg})
}
func (m Model) rpcCloseMessage() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionCloseMessage})
}
func (m Model) rpcToggleMark(messageID string, duplicate bool) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionToggleMark, MessageID: messageID, Duplicate: duplicate})
}
func (m Model) rpcShowActive() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionShowActiveQueue})
}
func (m Model) rpcShowDLQ() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionShowDeadLetterQueue})
}
func (m Model) rpcToggleDLQFilter() tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionToggleDLQFilter})
}
func (m Model) rpcRequeue(ids []string, selectedID string, duplicate bool) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionRequeue, MessageIDs: ids, MessageID: selectedID, Duplicate: duplicate})
}
func (m Model) rpcDeleteDuplicate(messageID string) tea.Cmd {
	return rpcActionCmd(m.client, sbcore.ActionRequest{Action: sbcore.ActionDeleteDuplicate, MessageID: messageID})
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

func (m Model) rpcDebugState() string {
	return fmt.Sprintf("rpc=%t session=%t", m.rpcEnabled(), m.sessionID != "")
}
