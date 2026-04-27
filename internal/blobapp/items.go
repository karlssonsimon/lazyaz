package blobapp

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

type accountItem struct {
	account blob.Account
}

func (i accountItem) Title() string {
	return i.account.Name
}

func (i accountItem) Description() string {
	return ""
}

func (i accountItem) FilterValue() string {
	return i.account.Name + " " + i.account.SubscriptionID + " " + i.account.ResourceGroup
}

type containerItem struct {
	container blob.ContainerInfo
}

func (i containerItem) Title() string {
	return i.container.Name
}

func (i containerItem) Description() string {
	return ""
}

func (i containerItem) FilterValue() string {
	return i.container.Name
}

type blobItem struct {
	blob         blob.BlobEntry
	displayName  string
	contentWidth int // pane content width for column alignment
}

func (i blobItem) Title() string {
	// The bubbles list default delegate truncates the rendered title
	// to (list.Width - NormalTitle.PaddingLeft - PaddingRight). Our
	// NormalTitle has 2 cells of left padding (no right padding), so
	// the title text gets a budget of contentWidth - 2. If we format
	// to the full contentWidth, the delegate trims the last 2 cells
	// and replaces them with `…`. Bake the padding awareness in here
	// so the row ends cleanly at the column edge.
	const delegateLeftPad = 2
	width := i.contentWidth - delegateLeftPad
	if i.contentWidth <= 0 {
		width = 80 - delegateLeftPad
	}
	if width < 1 {
		width = 1
	}
	marker := fileIcon(i.displayName)
	if i.blob.IsPrefix {
		marker = "+"
	}
	markerWidth := lipgloss.Width(marker + " ") // 2 cells

	// Below 34 effective cells (was 36 before delegateLeftPad
	// accounting) we drop the date+size columns and let the name take
	// what's left — the meta grid would be wider than the cell.
	if i.blob.IsPrefix || width < 34 {
		nameWidth := width - markerWidth
		if nameWidth < 1 {
			nameWidth = 1
		}
		return truncateToWidth(marker+" "+truncateMiddleRunes(i.displayName, nameWidth), width)
	}

	// Fixed grid that matches blobsColumnHeader so the row aligns
	// with the NAME / MODIFIED / SIZE labels regardless of how short
	// any individual row's size string happens to be. The name column
	// is capped at blobNameColMax so date+size don't drift far from
	// the name on wide focused columns; trailing space fills to width.
	const dateCol = blobDateColWidth // 16
	const sizeCol = blobSizeColWidth // 10
	const gaps = 4                   // two 2-cell gaps between columns
	date := formatDate(i.blob.LastModified)
	size := humanSize(i.blob.Size)

	nameCol := width - markerWidth - dateCol - sizeCol - gaps
	sizePad := sizeCol - lipgloss.Width(size)
	if nameCol < 4 {
		// Tight row: drop the size right-padding so the size column
		// shrinks to its actual width instead of pushing the line
		// past `width`. Misaligns wide-size rows with their narrower
		// neighbours, but only triggers when the column is very narrow.
		sizePad = 0
		nameCol = width - markerWidth - dateCol - lipgloss.Width(size) - gaps
		if nameCol < 1 {
			nameCol = 1
		}
	} else if nameCol > blobNameColMax {
		nameCol = blobNameColMax
	}
	if sizePad < 0 {
		sizePad = 0
	}

	name := truncateMiddleRunes(i.displayName, nameCol)
	namePad := nameCol - lipgloss.Width(name)
	if namePad < 0 {
		namePad = 0
	}
	body := fmt.Sprintf("%s %s%s  %s  %s%s",
		marker,
		name, strings.Repeat(" ", namePad),
		date,
		strings.Repeat(" ", sizePad), size,
	)
	if trailing := width - lipgloss.Width(body); trailing > 0 {
		body += strings.Repeat(" ", trailing)
	}
	return truncateToWidth(body, width)
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return "       -        "
	}
	return t.Local().Format("2006-01-02 15:04")
}

// blobsColumnHeader builds the NAME / MODIFIED / SIZE table header that
// sits between the column title and the first row. Layout matches
// blobItem.Title above so columns line up: 2-cell leading gutter (to
// match the list delegate's selection padding), name (capped), two
// spaces, 16-cell date, two spaces, right-aligned 10-cell size, then
// optional trailing space to reach `width`.
const (
	blobDateColWidth = 16
	blobSizeColWidth = 10
	blobNameColMax   = 50 // cap so date+size don't drift far from name on wide columns
)

func blobsColumnHeader(width int) string {
	if width < 36 {
		return ""
	}
	metaWidth := blobDateColWidth + blobSizeColWidth + 4 // 4 = two double-space gaps
	nameColWidth := width - 2 - metaWidth                // 2 = leading gutter
	if nameColWidth < 4 {
		return ""
	}
	if nameColWidth > blobNameColMax {
		nameColWidth = blobNameColMax
	}
	line := fmt.Sprintf("  %-*s  %-*s  %*s",
		nameColWidth, "NAME",
		blobDateColWidth, "MODIFIED",
		blobSizeColWidth, "SIZE")
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

func (i blobItem) Description() string {
	return ""
}

func (i blobItem) FilterValue() string {
	return i.displayName
}

func accountsToItems(accounts []blob.Account) []list.Item {
	items := make([]list.Item, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, accountItem{account: account})
	}
	return items
}

func containersToItems(containers []blob.ContainerInfo) []list.Item {
	items := make([]list.Item, 0, len(containers))
	for _, containerInfo := range containers {
		items = append(items, containerItem{container: containerInfo})
	}
	return items
}

// sortBlobs returns a sorted copy of entries. Prefixes (dirs) always appear
// before regular blobs. Within each group the chosen field and direction apply.
func sortBlobs(entries []blob.BlobEntry, field blobSortField, desc bool) []blob.BlobEntry {
	if field == blobSortNone {
		return entries
	}
	out := make([]blob.BlobEntry, len(entries))
	copy(out, entries)
	slices.SortStableFunc(out, func(a, b blob.BlobEntry) int {
		// Dirs before files, always.
		if a.IsPrefix != b.IsPrefix {
			if a.IsPrefix {
				return -1
			}
			return 1
		}
		var c int
		switch field {
		case blobSortName:
			c = cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		case blobSortSize:
			c = cmp.Compare(a.Size, b.Size)
		case blobSortDate:
			c = a.LastModified.Compare(b.LastModified)
		}
		if desc {
			c = -c
		}
		return c
	})
	return out
}

func blobsToItems(entries []blob.BlobEntry, prefix string, contentWidth int) []list.Item {
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, blobItem{
			blob:         entry,
			displayName:  trimPrefixForDisplay(entry.Name, prefix),
			contentWidth: contentWidth,
		})
	}
	return items
}

func truncateMiddleRunes(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth < 5 {
		return truncateToWidth(s, maxWidth)
	}
	endWidth := maxWidth * 2 / 5
	if endWidth < 2 {
		endWidth = 2
	}
	end := truncateLeftToWidth(s, endWidth)
	startWidth := maxWidth - lipgloss.Width(end) - 1
	if startWidth < 1 {
		startWidth = 1
		end = truncateLeftToWidth(s, maxWidth-startWidth-1)
	}
	return truncateToWidth(s, startWidth) + "…" + end
}

func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	var out strings.Builder
	width := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > maxWidth {
			break
		}
		out.WriteRune(r)
		width += rw
	}
	return out.String()
}

func truncateLeftToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	width := 0
	start := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		rw := lipgloss.Width(string(runes[i]))
		if width+rw > maxWidth {
			break
		}
		width += rw
		start = i
	}
	return string(runes[start:])
}

func trimPrefixForDisplay(name, prefix string) string {
	if prefix == "" {
		return name
	}
	trimmed := strings.TrimPrefix(name, prefix)
	if trimmed == "" {
		return name
	}
	return trimmed
}

func displayPrefix(prefix string) string {
	if prefix == "" {
		return "/"
	}
	return prefix
}

func parentPrefix(prefix string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	idx := strings.LastIndex(prefix, "/")
	if idx < 0 {
		return ""
	}
	return prefix[:idx+1]
}

// Identity functions used by cache.Broker's internal merge and
// ui.SetItemsPreserveKey. Blob prefixes (synthetic "folder" entries in
// hierarchy mode) use the same Name field as real blobs, so a single
// keyer handles both cases.

func accountKey(a blob.Account) string         { return a.Name }
func containerKey(c blob.ContainerInfo) string { return c.Name }
func blobEntryKey(b blob.BlobEntry) string     { return b.Name }

func accountItemKey(it list.Item) string {
	if ai, ok := it.(accountItem); ok {
		return ai.account.Name
	}
	return ""
}

func containerItemKey(it list.Item) string {
	if ci, ok := it.(containerItem); ok {
		return ci.container.Name
	}
	return ""
}

func blobItemKey(it list.Item) string {
	if bi, ok := it.(blobItem); ok {
		return bi.blob.Name
	}
	return ""
}

func fileIcon(name string) string {
	ext := strings.ToLower(name)
	if i := strings.LastIndex(ext, "."); i >= 0 {
		ext = ext[i:]
	} else {
		ext = ""
	}
	switch ext {
	case ".json", ".yaml", ".yml", ".toml", ".xml", ".ini", ".cfg", ".conf":
		return "⚙"
	case ".go", ".py", ".js", ".ts", ".rs", ".java", ".c", ".cpp", ".cs", ".rb", ".sh", ".bash":
		return "◇"
	case ".md", ".txt", ".csv", ".log", ".rst":
		return "☰"
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".bmp", ".webp", ".ico", ".tiff":
		return "▣"
	case ".zip", ".gz", ".tar", ".bz2", ".7z", ".rar", ".zst", ".xz":
		return "▤"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return "▧"
	case ".mp3", ".wav", ".flac", ".ogg", ".aac", ".mp4", ".avi", ".mkv", ".mov", ".webm":
		return "▶"
	case ".parquet", ".avro", ".orc", ".db", ".sqlite":
		return "◫"
	default:
		return "◻"
	}
}

// blobListFilter wraps list.DefaultFilter and adjusts matched character
// indices so the underline lands on the correct characters in the
// title. Two adjustments are needed:
//  1. sahilm/fuzzy returns byte indices but lipgloss.StyleRunes
//     expects rune indices — convert via RuneCount.
//  2. Blob titles start with "icon " (2 runes) that is not part of
//     FilterValue — shift every index by 2.
func blobListFilter(term string, targets []string) []list.Rank {
	ranks := list.DefaultFilter(term, targets)
	for i, r := range ranks {
		target := targets[r.Index]
		for j, byteIdx := range r.MatchedIndexes {
			runeIdx := utf8.RuneCountInString(target[:byteIdx])
			ranks[i].MatchedIndexes[j] = runeIdx + 2
		}
	}
	return ranks
}

func blobSearchPrefix(currentPrefix, query string) string {
	needle := strings.TrimSpace(strings.ReplaceAll(query, "\\", "/"))
	if needle == "" {
		return strings.TrimSpace(currentPrefix)
	}
	if strings.HasPrefix(needle, "/") {
		return strings.TrimPrefix(needle, "/")
	}
	base := strings.TrimSpace(currentPrefix)
	if base == "" || strings.HasPrefix(needle, base) {
		return needle
	}
	return base + needle
}
