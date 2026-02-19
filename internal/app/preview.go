package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"azure-storage/internal/azure"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	previewBufferViewports = 10
	previewApproxLineBytes = 96
	previewMinWindowBytes  = int64(64 * 1024)
	previewMaxWindowBytes  = int64(2 * 1024 * 1024)
)

type previewState struct {
	open        bool
	blobName    string
	blobSize    int64
	contentType string
	language    string
	binary      bool
	cursor      int64
	windowStart int64
	windowData  []byte
	lineStarts  []int
	rendered    string
	requestID   int
	viewport    viewport.Model
}

var (
	previewTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	previewMetaStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	previewTagStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFilterMatch))
	jsonKeyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))
	jsonStringStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccentStrong))
	jsonNumberStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFilterMatch))
	jsonNullStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDanger))
	jsonPunctStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	xmlTagStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))
	xmlAttrStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFilterMatch))
	xmlPunctStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	csvCellStyleA     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	csvCellStyleB     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccentStrong))
	xmlTagPattern     = regexp.MustCompile(`<(?:"[^"]*"|'[^']*'|[^'">])*>`)
	xmlAttrPattern    = regexp.MustCompile(`\s([A-Za-z_:][A-Za-z0-9_.:-]*)=`)
)

func newPreviewState() previewState {
	vp := viewport.New(40, 10)
	vp.SetContent("")
	return previewState{viewport: vp}
}

func (m *Model) resetPreviewState() {
	m.preview = newPreviewState()
	m.pendingPreviewG = false
	if m.focus == previewPane {
		m.focus = blobsPane
	}
}

func (p previewState) title() string {
	if !p.open {
		return "Preview"
	}
	label := trimToWidth(p.blobName, 50)
	meta := fmt.Sprintf("%s | %s", emptyToDash(p.language), humanSize(p.blobSize))
	if p.binary {
		meta += " | binary"
	}
	return previewTitleStyle.Render("Preview") + " " + previewMetaStyle.Render(label+" | "+meta)
}

func (m Model) openPreview(blob azure.BlobEntry) (Model, tea.Cmd) {
	if blob.IsPrefix {
		m.status = "Open a blob file to preview"
		return m, nil
	}

	if !m.preview.open {
		m.preview.open = true
	}
	m.preview.blobName = blob.Name
	m.preview.blobSize = blob.Size
	m.preview.contentType = blob.ContentType
	m.preview.language = detectPreviewLanguage(blob.Name, blob.ContentType)
	m.preview.binary = false
	m.preview.cursor = 0
	m.preview.windowStart = 0
	m.preview.windowData = nil
	m.preview.lineStarts = nil
	m.preview.rendered = previewMetaStyle.Render("Loading preview...")
	m.preview.requestID++
	m.pendingPreviewG = false
	m.focus = previewPane
	m.loading = true
	m.lastErr = ""
	m.status = fmt.Sprintf("Loading preview for %s", blob.Name)
	m.resize()

	cmd := loadPreviewWindowCmd(
		m.service,
		m.currentAccount,
		m.containerName,
		blob.Name,
		0,
		blob.Size,
		blob.ContentType,
		max(1, m.preview.viewport.Height),
		m.preview.requestID,
	)
	return m, tea.Batch(spinner.Tick, cmd)
}

func (m Model) handlePreviewWindowLoaded(msg previewWindowLoadedMsg) (Model, tea.Cmd) {
	if !m.preview.open || !m.hasAccount || !m.hasContainer {
		return m, nil
	}
	if !sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container || m.preview.blobName != msg.blobName {
		return m, nil
	}
	if msg.requestID != m.preview.requestID {
		return m, nil
	}

	m.loading = false
	if msg.err != nil {
		m.lastErr = msg.err.Error()
		m.status = fmt.Sprintf("Failed to load preview for %s", msg.blobName)
		return m, nil
	}

	m.lastErr = ""
	m.preview.blobSize = msg.blobSize
	if strings.TrimSpace(msg.contentType) != "" {
		m.preview.contentType = msg.contentType
	}
	m.preview.windowStart = msg.windowStart
	m.preview.windowData = msg.data
	m.preview.cursor = clampInt64(msg.cursor, 0, maxInt64(0, msg.blobSize-1))
	m.preview.binary = isProbablyBinary(msg.data)
	m.preview.language = detectPreviewLanguage(msg.blobName, m.preview.contentType)
	m.preview.lineStarts = computeLineStarts(msg.data)
	m.preview.rendered = renderPreviewContent(msg.data, m.preview.language, m.preview.binary)
	m.preview.viewport.SetContent(m.preview.rendered)
	m.preview.viewport.YOffset = m.previewLocalLine()

	if m.preview.binary {
		m.status = fmt.Sprintf("Binary preview for %s (%s)", msg.blobName, humanSize(msg.blobSize))
	} else {
		m.status = fmt.Sprintf("Previewing %s (%s)", msg.blobName, humanSize(msg.blobSize))
	}

	return m, nil
}

func (m Model) handlePreviewKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "h", "left", "esc":
		m.pendingPreviewG = false
		m.focus = blobsPane
		m.status = "Focus: blobs"
		return m, nil
	case "tab":
		m.pendingPreviewG = false
		m.nextFocus()
		return m, nil
	case "shift+tab":
		m.pendingPreviewG = false
		m.previousFocus()
		return m, nil
	case "j", "down":
		m.pendingPreviewG = false
		return m.movePreviewCursorByLines(1)
	case "k", "up":
		m.pendingPreviewG = false
		return m.movePreviewCursorByLines(-1)
	case "ctrl+d":
		m.pendingPreviewG = false
		step := max(1, m.preview.viewport.Height/2)
		return m.movePreviewCursorByLines(step)
	case "ctrl+u":
		m.pendingPreviewG = false
		step := max(1, m.preview.viewport.Height/2)
		return m.movePreviewCursorByLines(-step)
	case "G":
		m.pendingPreviewG = false
		return m.jumpPreviewToBottom()
	case "g":
		if m.pendingPreviewG {
			m.pendingPreviewG = false
			return m.jumpPreviewToTop()
		}
		m.pendingPreviewG = true
		m.status = "Press g again for top"
		return m, nil
	default:
		m.pendingPreviewG = false
		return m, nil
	}
}

func (m Model) jumpPreviewToTop() (Model, tea.Cmd) {
	m.preview.cursor = 0
	return m.ensurePreviewWindowAtCursor()
}

func (m Model) jumpPreviewToBottom() (Model, tea.Cmd) {
	if m.preview.blobSize <= 0 {
		m.preview.cursor = 0
	} else {
		m.preview.cursor = m.preview.blobSize - 1
	}
	return m.ensurePreviewWindowAtCursor()
}

func (m Model) movePreviewCursorByLines(delta int) (Model, tea.Cmd) {
	if !m.preview.open || delta == 0 {
		return m, nil
	}
	if len(m.preview.windowData) == 0 || len(m.preview.lineStarts) == 0 {
		return m.ensurePreviewWindowAtCursor()
	}

	local := m.previewLocalLine()
	target := local + delta
	if target < 0 {
		target = 0
	}
	if target >= len(m.preview.lineStarts) {
		target = len(m.preview.lineStarts) - 1
	}

	m.preview.cursor = m.preview.windowStart + int64(m.preview.lineStarts[target])
	if m.preview.blobSize > 0 {
		m.preview.cursor = clampInt64(m.preview.cursor, 0, m.preview.blobSize-1)
	}
	return m.ensurePreviewWindowAtCursor()
}

func (m Model) ensurePreviewWindowAtCursor() (Model, tea.Cmd) {
	windowEnd := m.preview.windowStart + int64(len(m.preview.windowData))
	needLoad := false

	if len(m.preview.windowData) == 0 || m.preview.cursor < m.preview.windowStart || m.preview.cursor >= windowEnd {
		needLoad = true
	}

	if !needLoad && len(m.preview.lineStarts) > 0 {
		visible := max(1, m.preview.viewport.Height)
		local := m.previewLocalLine()
		if m.preview.windowStart > 0 && local < visible*previewBufferViewports {
			needLoad = true
		}
		if windowEnd < m.preview.blobSize && local > len(m.preview.lineStarts)-visible*(previewBufferViewports+1) {
			needLoad = true
		}
	}

	if needLoad {
		m.preview.requestID++
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading preview window for %s", m.preview.blobName)
		cmd := loadPreviewWindowCmd(
			m.service,
			m.currentAccount,
			m.containerName,
			m.preview.blobName,
			m.preview.cursor,
			m.preview.blobSize,
			m.preview.contentType,
			max(1, m.preview.viewport.Height),
			m.preview.requestID,
		)
		return m, tea.Batch(spinner.Tick, cmd)
	}

	m.preview.viewport.YOffset = m.previewLocalLine()
	return m, nil
}

func (m Model) previewLocalLine() int {
	if len(m.preview.lineStarts) == 0 {
		return 0
	}
	localOffset := int(clampInt64(m.preview.cursor-m.preview.windowStart, 0, int64(len(m.preview.windowData))))
	idx := sort.Search(len(m.preview.lineStarts), func(i int) bool {
		return m.preview.lineStarts[i] > localOffset
	})
	if idx == 0 {
		return 0
	}
	line := idx - 1
	if line >= len(m.preview.lineStarts) {
		return len(m.preview.lineStarts) - 1
	}
	return line
}

func computeLineStarts(data []byte) []int {
	if len(data) == 0 {
		return []int{0}
	}
	starts := []int{0}
	for i, b := range data {
		if b == '\n' && i+1 <= len(data) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func loadPreviewWindowCmd(
	svc *azure.Service,
	account azure.Account,
	containerName string,
	blobName string,
	cursor int64,
	knownSize int64,
	knownContentType string,
	visibleLines int,
	requestID int,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		size := knownSize
		contentType := knownContentType
		if size <= 0 || strings.TrimSpace(contentType) == "" {
			props, err := svc.GetBlobProperties(ctx, account, containerName, blobName)
			if err != nil {
				return previewWindowLoadedMsg{
					requestID: requestID,
					account:   account,
					container: containerName,
					blobName:  blobName,
					err:       err,
				}
			}
			size = props.Size
			if strings.TrimSpace(contentType) == "" {
				contentType = props.ContentType
			}
		}

		windowStart, windowCount := computePreviewWindow(size, cursor, visibleLines)
		data, err := svc.ReadBlobRange(ctx, account, containerName, blobName, windowStart, windowCount)
		return previewWindowLoadedMsg{
			requestID:   requestID,
			account:     account,
			container:   containerName,
			blobName:    blobName,
			blobSize:    size,
			contentType: contentType,
			windowStart: windowStart,
			cursor:      cursor,
			data:        data,
			err:         err,
		}
	}
}

func computePreviewWindow(totalSize, cursor int64, visibleLines int) (int64, int64) {
	if totalSize <= 0 {
		return 0, 0
	}

	visibleBytes := int64(max(1, visibleLines) * previewApproxLineBytes)
	bufferBytes := visibleBytes * previewBufferViewports
	windowSize := visibleBytes + 2*bufferBytes
	if windowSize < previewMinWindowBytes {
		windowSize = previewMinWindowBytes
	}
	if windowSize > previewMaxWindowBytes {
		windowSize = previewMaxWindowBytes
	}
	if windowSize > totalSize {
		windowSize = totalSize
	}

	anchored := clampInt64(cursor, 0, maxInt64(0, totalSize-1))
	start := anchored - bufferBytes
	if start < 0 {
		start = 0
	}
	if start+windowSize > totalSize {
		start = maxInt64(0, totalSize-windowSize)
	}

	return start, windowSize
}

func detectPreviewLanguage(blobName, contentType string) string {
	lowerName := strings.ToLower(blobName)
	lowerType := strings.ToLower(contentType)

	switch {
	case strings.HasSuffix(lowerName, ".json") || strings.Contains(lowerType, "json"):
		return "json"
	case strings.HasSuffix(lowerName, ".xml") || strings.Contains(lowerType, "xml"):
		return "xml"
	case strings.HasSuffix(lowerName, ".csv") || strings.Contains(lowerType, "csv"):
		return "csv"
	default:
		return "text"
	}
}

func renderPreviewContent(data []byte, language string, binary bool) string {
	if binary {
		return previewTagStyle.Render("Binary content preview is not supported.")
	}

	if len(data) == 0 {
		return previewMetaStyle.Render("(empty blob)")
	}

	text := string(data)
	switch language {
	case "json":
		return highlightJSONPreview(text)
	case "xml":
		return highlightXMLPreview(text)
	case "csv":
		return highlightCSVPreview(text)
	default:
		return text
	}
}

func highlightJSONPreview(input string) string {
	var buf strings.Builder
	formatted := input
	var indented bytes.Buffer
	if json.Indent(&indented, []byte(input), "", "  ") == nil {
		formatted = indented.String()
	}

	lines := strings.Split(formatted, "\n")
	for i, line := range lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(colorizeJSONLine(line))
	}

	return buf.String()
}

func colorizeJSONLine(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	if trimmed == "" {
		return line
	}

	var out strings.Builder
	out.WriteString(indent)
	if strings.HasPrefix(trimmed, "\"") {
		colonIdx := strings.Index(trimmed, "\":")
		if colonIdx > 0 {
			out.WriteString(jsonKeyStyle.Render(trimmed[:colonIdx+1]))
			out.WriteString(jsonPunctStyle.Render(":"))
			val := strings.TrimSpace(trimmed[colonIdx+2:])
			if val != "" {
				out.WriteByte(' ')
				out.WriteString(colorizeJSONValue(val))
			}
			return out.String()
		}
	}
	out.WriteString(colorizeJSONValue(trimmed))
	return out.String()
}

func colorizeJSONValue(v string) string {
	trailing := ""
	clean := v
	for strings.HasSuffix(clean, ",") {
		trailing = "," + trailing
		clean = strings.TrimSuffix(clean, ",")
	}

	var styled string
	switch {
	case clean == "{" || clean == "}" || clean == "[" || clean == "]" || clean == "{}" || clean == "[]":
		styled = jsonPunctStyle.Render(clean)
	case clean == "null":
		styled = jsonNullStyle.Render(clean)
	case clean == "true" || clean == "false":
		styled = jsonNumberStyle.Render(clean)
	case strings.HasPrefix(clean, "\""):
		styled = jsonStringStyle.Render(clean)
	default:
		styled = jsonNumberStyle.Render(clean)
	}

	if trailing != "" {
		return styled + jsonPunctStyle.Render(trailing)
	}
	return styled
}

func highlightXMLPreview(input string) string {
	return xmlTagPattern.ReplaceAllStringFunc(input, func(tag string) string {
		colored := xmlTagStyle.Render(tag)
		colored = xmlAttrPattern.ReplaceAllString(colored, " "+xmlAttrStyle.Render("$1")+xmlPunctStyle.Render("="))
		return colored
	})
}

func highlightCSVPreview(input string) string {
	lines := strings.Split(input, "\n")
	var out strings.Builder
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		for idx, part := range parts {
			if idx > 0 {
				out.WriteString(xmlPunctStyle.Render(","))
			}
			if idx%2 == 0 {
				out.WriteString(csvCellStyleA.Render(part))
			} else {
				out.WriteString(csvCellStyleB.Render(part))
			}
		}
	}
	return out.String()
}

func isProbablyBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if strings.Contains(string(data), "\x00") {
		return true
	}
	control := 0
	for _, b := range data {
		if b < 0x09 {
			control++
			continue
		}
		if b > 0x0D && b < 0x20 {
			control++
		}
	}
	return float64(control)/float64(len(data)) > 0.05
}

func clampInt64(v, minVal, maxVal int64) int64 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
