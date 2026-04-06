package sbapp

import "azure-storage/internal/azure/servicebus"

func (m *Model) clearDetailState() {
	m.topicSubs = nil
	m.peekedMessages = nil
	m.viewingTopicSub = false
	m.currentTopicSub = servicebus.TopicSubscription{}
	m.detailMode = detailMessages
	m.deadLetter = false
	m.markedMessages = make(map[string]struct{})
	m.duplicateMessages = make(map[string]struct{})
}

func (m Model) collectRequeueIDs() []string {
	if len(m.markedMessages) > 0 {
		var ids []string
		for _, msg := range m.peekedMessages {
			if _, ok := m.markedMessages[msg.MessageID]; !ok {
				continue
			}
			if _, isDup := m.duplicateMessages[msg.MessageID]; isDup {
				continue
			}
			ids = append(ids, msg.MessageID)
		}
		return ids
	}
	item, ok := m.detailList.SelectedItem().(messageItem)
	if !ok || item.duplicate {
		return nil
	}
	return []string{item.message.MessageID}
}

func (m *Model) refreshItems() {
	idx := m.detailList.Index()
	m.detailList.SetItems(messagesToItems(m.peekedMessages, m.markedMessages, m.duplicateMessages))
	m.detailList.Select(idx)
}
