package sbapp

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"
)

// The message pane shows one of three views, mirroring the portal's
// Service Bus Explorer tabs: the body, the broker properties, and the
// sender's custom (application) properties. The property views are
// plain text tables fed through the same TextBuffer seam as the body,
// so scrolling, search, the vim capture and yank all work on them
// unchanged — msgBody() is the single switch.

type msgViewKind int

const (
	msgViewBody msgViewKind = iota
	msgViewBroker
	msgViewCustom
)

func (v msgViewKind) String() string {
	switch v {
	case msgViewBroker:
		return "broker properties"
	case msgViewCustom:
		return "custom properties"
	}
	return "body"
}

// next is the cycle order: body → broker → custom → body.
func (v msgViewKind) next() msgViewKind {
	return (v + 1) % 3
}

// propRow is one line of a property table.
type propRow struct {
	label, value string
}

// brokerPropertyRows is the fixed broker set, dashes for anything the
// broker did not set. The dead-letter fields come last so the
// interesting part of a DLQ message sits together at the bottom.
func brokerPropertyRows(msg servicebus.PeekedMessage) []propRow {
	return []propRow{
		{"Message ID", ui.EmptyToDash(msg.MessageID)},
		{"Sequence Number", propInt(msg.SequenceNumber)},
		{"Enqueued Sequence Number", propInt(msg.EnqueuedSequenceNumber)},
		{"Enqueued At", propTime(msg.EnqueuedAt)},
		{"Scheduled Enqueue Time", propTime(msg.ScheduledEnqueueTime)},
		{"Expires At", propTime(msg.ExpiresAt)},
		{"Time To Live", propDuration(msg.TimeToLive)},
		{"Delivery Count", fmt.Sprintf("%d", msg.DeliveryCount)},
		{"State", ui.EmptyToDash(msg.State)},
		{"Content Type", ui.EmptyToDash(msg.ContentType)},
		{"Subject", ui.EmptyToDash(msg.Subject)},
		{"Correlation ID", ui.EmptyToDash(msg.CorrelationID)},
		{"Session ID", ui.EmptyToDash(msg.SessionID)},
		{"Partition Key", ui.EmptyToDash(msg.PartitionKey)},
		{"Reply To", ui.EmptyToDash(msg.ReplyTo)},
		{"Reply To Session ID", ui.EmptyToDash(msg.ReplyToSessionID)},
		{"To", ui.EmptyToDash(msg.To)},
		{"Locked Until", propTime(msg.LockedUntil)},
		{"Lock Token", ui.EmptyToDash(msg.LockToken)},
		{"Dead-Letter Source", ui.EmptyToDash(msg.DeadLetterSource)},
		{"Dead-Letter Reason", ui.EmptyToDash(msg.DeadLetterReason)},
		{"Dead-Letter Description", ui.EmptyToDash(msg.DeadLetterDescription)},
	}
}

// customPropertyRows is the sender's application properties, keys
// sorted so the table is stable between messages.
func customPropertyRows(msg servicebus.PeekedMessage) []propRow {
	keys := sortedKeys(msg.AppProperties)
	rows := make([]propRow, len(keys))
	for i, k := range keys {
		rows[i] = propRow{k, msg.AppProperties[k]}
	}
	return rows
}

// noCustomProperties is the custom view's whole text when the sender
// set nothing; the yank paths check for it so it never hits the
// clipboard.
const noCustomProperties = "(no custom properties)"

func brokerPropertiesText(msg servicebus.PeekedMessage) string {
	return renderPropRows(brokerPropertyRows(msg))
}

func customPropertiesText(msg servicebus.PeekedMessage) string {
	rows := customPropertyRows(msg)
	if len(rows) == 0 {
		return noCustomProperties
	}
	return renderPropRows(rows)
}

// renderPropRows lays the rows out as two aligned columns. A value
// spanning several lines keeps its line breaks and indents the
// continuation under the value column, so the table stays a table.
func renderPropRows(rows []propRow) string {
	width := 0
	for _, r := range rows {
		if n := len([]rune(r.label)); n > width {
			width = n
		}
	}
	indent := strings.Repeat(" ", width+2)
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(r.label)
		b.WriteString(strings.Repeat(" ", width-len([]rune(r.label))+2))
		b.WriteString(strings.ReplaceAll(r.value, "\n", "\n"+indent))
	}
	return b.String()
}

// styledPropRows is renderPropRows with the label column muted. Only
// styling is added, so the columns line up with the plain text every
// other path (search, cursor, yank) computes over.
func styledPropRows(rows []propRow, styles ui.Styles) string {
	if len(rows) == 0 {
		return styles.Muted.Render(noCustomProperties)
	}
	plain := renderPropRows(rows)
	width := 0
	for _, r := range rows {
		if n := len([]rune(r.label)); n > width {
			width = n
		}
	}
	lines := strings.Split(plain, "\n")
	for i, line := range lines {
		rl := []rune(line)
		if len(rl) < width+2 || strings.TrimSpace(string(rl[:width])) == "" {
			continue // continuation line, no label
		}
		lines[i] = styles.Muted.Render(string(rl[:width])) + string(rl[width:])
	}
	return strings.Join(lines, "\n")
}

func propInt(n int64) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func propTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05 MST")
}

func propDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	return d.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// msgViewportContent renders whatever msgBody() returns for the pane:
// syntax-highlighted for the body, label-muted for the tables.
func (m Model) msgViewportContent() string {
	switch m.msgView {
	case msgViewBroker:
		return styledPropRows(brokerPropertyRows(m.selectedMessage), m.Styles)
	case msgViewCustom:
		return styledPropRows(customPropertyRows(m.selectedMessage), m.Styles)
	}
	return m.Styles.Syntax.HighlightJSON(m.msgBody())
}

// setMsgViewportContent pushes the current view into the viewport. It
// leaves the scroll position alone; callers decide whether to reset.
func (m *Model) setMsgViewportContent() {
	m.messageViewport.SetContent(m.msgViewportContent())
}

// cycleMsgView is the properties key: body → broker → custom → body.
// Cursor, search and selection reset — the views share no positions.
// The vim capture, if active, survives the switch like it does for =.
func (m Model) cycleMsgView() (Model, tea.Cmd) {
	m.msgView = m.msgView.next()
	m.msgVim.cur = vim.Cursor{}
	m.msgVim.span.Stop()
	m.messageSearch.bar.Clear()
	m.messageSearch.cursor = 0
	m.setMsgViewportContent()
	m.messageViewport.SetYOffset(0)
	m.messageViewport.SetXOffset(0)
	m.Notify(appshell.LevelInfo, "Showing "+m.msgView.String())
	return m, nil
}

// yankMsgView is y in browse mode: the whole of whatever is shown.
func (m Model) yankMsgView() (Model, tea.Cmd) {
	if m.msgView == msgViewCustom && len(m.selectedMessage.AppProperties) == 0 {
		m.Notify(appshell.LevelInfo, "Message has no custom properties")
		return m, nil
	}
	return m.yankMessageBody(m.msgBody())
}
