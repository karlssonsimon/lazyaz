package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
)

type Pane int

const (
	SubscriptionsPane Pane = iota
	AccountsPane
	ContainersPane
	BlobsPane
	PreviewPane
)

type LeftAction int

const (
	LeftNoop LeftAction = iota
	LeftHierarchyUp
	LeftFocusBlobs
	LeftFocusContainers
	LeftFocusAccounts
	LeftFocusSubscriptions
)

type LoadKind int

const (
	LoadNone LoadKind = iota
	LoadSubscriptions
	LoadAccounts
	LoadContainers
	LoadHierarchyBlobs
	LoadAllBlobs
	LoadSearchBlobs
	LoadDownload
	LoadPreviewWindow
)

type LoadRequest struct {
	Kind             LoadKind
	Force            bool
	SubscriptionID   string
	Account          blob.Account
	ContainerName    string
	Prefix           string
	Query            string
	BlobName         string
	BlobNames        []string
	DestinationRoot  string
	Status           string
	Limit            int
	VisibleLines     int
	Cursor           int64
	KnownSize        int64
	KnownContentType string
}

type OpenBlobResult struct {
	Status      string
	LoadRequest LoadRequest
	OpenPreview bool
}

type State struct {
	Focus               Pane
	HasSubscription     bool
	CurrentSubscription azure.Subscription
	HasAccount          bool
	CurrentAccount      blob.Account
	HasContainer        bool
	ContainerName       string
	Prefix              string
	BlobLoadAll         bool
	BlobSearchQuery     string
	PreviewOpen         bool
}

type Session struct {
	State
	Subscriptions  []azure.Subscription
	Accounts       []blob.Account
	Containers     []blob.ContainerInfo
	Blobs          []blob.BlobEntry
	MarkedBlobs    map[string]blob.BlobEntry
	VisualLineMode bool
	VisualAnchor   string
	Preview        Preview
	Loading        bool
	Status         string
	LastErr        string
}

type Preview struct {
	Open        bool
	BlobName    string
	BlobSize    int64
	ContentType string
	Binary      bool
	Cursor      int64
	WindowStart int64
	WindowData  []byte
	LineStarts  []int
	RequestID   int
}

func NewState() State {
	return State{Focus: SubscriptionsPane}
}

func NewSession() Session {
	return Session{State: NewState(), MarkedBlobs: make(map[string]blob.BlobEntry)}
}

func (s *Session) BeginLoading(status string) {
	s.Loading = true
	s.LastErr = ""
	s.Status = status
}

func (s *Session) ClearError() {
	s.LastErr = ""
}

func (s *Session) SetError(status string, err error) {
	s.Loading = false
	s.Status = status
	if err == nil {
		s.LastErr = ""
		return
	}
	s.LastErr = err.Error()
}

func (s *Session) SetStatus(status string) {
	s.Status = status
}

func (s *Session) SelectSubscription(sub azure.Subscription) {
	s.State.SelectSubscription(sub)
	s.Accounts = nil
	s.Containers = nil
	s.Blobs = nil
	s.ClearSelections()
	s.ResetPreview()
}

func (s *Session) SelectAccount(account blob.Account) {
	s.State.SelectAccount(account)
	s.Containers = nil
	s.Blobs = nil
	s.ClearSelections()
	s.ResetPreview()
}

func (s *Session) SelectContainer(name string) {
	s.State.SelectContainer(name)
	s.Blobs = nil
	s.ClearSelections()
	s.ResetPreview()
}

func (s *Session) ClearSelections() {
	s.VisualLineMode = false
	s.VisualAnchor = ""
	if s.MarkedBlobs == nil {
		s.MarkedBlobs = make(map[string]blob.BlobEntry)
		return
	}
	for name := range s.MarkedBlobs {
		delete(s.MarkedBlobs, name)
	}
}

func (s *Session) ExitVisualMode() string {
	s.VisualLineMode = false
	s.VisualAnchor = ""
	return fmt.Sprintf("Visual mode off. %d marked with space.", len(s.MarkedBlobs))
}

func (s *Session) ToggleVisualLineMode(currentName string, selectionCount int) string {
	s.VisualLineMode = !s.VisualLineMode
	if !s.VisualLineMode {
		s.VisualAnchor = ""
		return fmt.Sprintf("Visual mode off. %d marked with space.", len(s.MarkedBlobs))
	}
	s.VisualAnchor = currentName
	if s.VisualAnchor == "" {
		return "Visual mode on. Move up/down to select a range."
	}
	return fmt.Sprintf("Visual mode on. %d in range.", selectionCount)
}

func (s *Session) ToggleMarkedBlob(entry blob.BlobEntry, displayName string) string {
	if entry.IsPrefix {
		return "Folder selection is not supported yet"
	}
	if s.MarkedBlobs == nil {
		s.MarkedBlobs = make(map[string]blob.BlobEntry)
	}
	if _, exists := s.MarkedBlobs[entry.Name]; exists {
		delete(s.MarkedBlobs, entry.Name)
		return fmt.Sprintf("Unmarked %s (%d marked)", displayName, len(s.MarkedBlobs))
	}
	s.MarkedBlobs[entry.Name] = entry
	return fmt.Sprintf("Marked %s (%d marked)", displayName, len(s.MarkedBlobs))
}

func (s Session) SortedMarkedBlobNames() []string {
	if len(s.MarkedBlobs) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.MarkedBlobs))
	for name := range s.MarkedBlobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Session) OpenPrefix(prefix string) {
	s.State.OpenPrefix(prefix)
}

func (s *Session) RefreshRequest(hierarchyLimit int, prefixSearchLimit int) LoadRequest {
	if !s.HasSubscription {
		return LoadRequest{Kind: LoadSubscriptions, Force: true, Status: "Refreshing subscriptions..."}
	}
	if !s.HasAccount || s.Focus == AccountsPane {
		return LoadRequest{Kind: LoadAccounts, Force: true, SubscriptionID: s.CurrentSubscription.ID, Status: fmt.Sprintf("Loading storage accounts in %s", subscriptionDisplayName(s.CurrentSubscription))}
	}
	if s.Focus == ContainersPane || !s.HasContainer {
		return LoadRequest{Kind: LoadContainers, Force: true, Account: s.CurrentAccount, Status: fmt.Sprintf("Loading containers in %s", s.CurrentAccount.Name)}
	}
	if s.Focus == PreviewPane && s.Preview.Open {
		return s.PreviewWindowRequest(1)
	}
	if s.BlobLoadAll {
		return LoadRequest{Kind: LoadAllBlobs, Force: true, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Status: fmt.Sprintf("Loading all blobs in %s/%s", s.CurrentAccount.Name, s.ContainerName)}
	}
	if s.BlobSearchQuery != "" {
		return LoadRequest{Kind: LoadSearchBlobs, Force: true, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Query: s.BlobSearchQuery, Limit: prefixSearchLimit, Status: fmt.Sprintf("Searching blobs by prefix %q...", BlobSearchPrefix(s.Prefix, s.BlobSearchQuery))}
	}
	return LoadRequest{Kind: LoadHierarchyBlobs, Force: true, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Limit: hierarchyLimit, Status: fmt.Sprintf("Loading up to %d entries under %q", hierarchyLimit, s.Prefix)}
}

func (s *Session) SelectSubscriptionRequest(sub azure.Subscription) LoadRequest {
	s.SelectSubscription(sub)
	return LoadRequest{Kind: LoadAccounts, SubscriptionID: sub.ID, Status: fmt.Sprintf("Loading storage accounts in %s", subscriptionDisplayName(sub))}
}

func (s *Session) SelectAccountRequest(account blob.Account) LoadRequest {
	s.SelectAccount(account)
	return LoadRequest{Kind: LoadContainers, Account: account, Status: fmt.Sprintf("Loading containers in %s", account.Name)}
}

func (s *Session) SelectContainerRequest(name string, hierarchyLimit int) LoadRequest {
	s.SelectContainer(name)
	return LoadRequest{Kind: LoadHierarchyBlobs, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Limit: hierarchyLimit, Status: fmt.Sprintf("Loading up to %d entries in %s/%s", hierarchyLimit, s.CurrentAccount.Name, s.ContainerName)}
}

func (s *Session) OpenPrefixRequest(prefix string, hierarchyLimit int) LoadRequest {
	s.OpenPrefix(prefix)
	return LoadRequest{Kind: LoadHierarchyBlobs, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Limit: hierarchyLimit, Status: fmt.Sprintf("Loading up to %d entries under %q", hierarchyLimit, s.Prefix)}
}

func (s *Session) OpenBlobEntry(entry blob.BlobEntry, hierarchyLimit int, visibleLines int) OpenBlobResult {
	if entry.IsPrefix {
		if s.BlobLoadAll {
			return OpenBlobResult{Status: "Directory navigation is unavailable when all blobs are loaded"}
		}
		return OpenBlobResult{LoadRequest: s.OpenPrefixRequest(entry.Name, hierarchyLimit)}
	}
	s.BeginPreview(entry)
	return OpenBlobResult{LoadRequest: s.PreviewWindowRequest(visibleLines), OpenPreview: true}
}

func (s *Session) ToggleLoadAllRequest(hierarchyLimit int) LoadRequest {
	s.BlobSearchQuery = ""
	if s.BlobLoadAll {
		s.BlobLoadAll = false
		return LoadRequest{Kind: LoadHierarchyBlobs, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Limit: hierarchyLimit, Status: fmt.Sprintf("Loading up to %d entries under %q", hierarchyLimit, s.Prefix)}
	}
	s.BlobLoadAll = true
	return LoadRequest{Kind: LoadAllBlobs, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Status: fmt.Sprintf("Loading all blobs in %s/%s", s.CurrentAccount.Name, s.ContainerName)}
}

func (s *Session) NavigateLeftRequest(hierarchyLimit int) (LeftAction, LoadRequest) {
	action := s.NavigateLeft()
	if action == LeftHierarchyUp {
		return action, LoadRequest{Kind: LoadHierarchyBlobs, Account: s.CurrentAccount, ContainerName: s.ContainerName, Prefix: s.Prefix, Limit: hierarchyLimit, Status: fmt.Sprintf("Loading up to %d entries under %q", hierarchyLimit, s.Prefix)}
	}
	return action, LoadRequest{}
}

func (s *Session) PreviewWindowRequest(visibleLines int) LoadRequest {
	return LoadRequest{
		Kind:             LoadPreviewWindow,
		Account:          s.CurrentAccount,
		ContainerName:    s.ContainerName,
		BlobName:         s.Preview.BlobName,
		Status:           fmt.Sprintf("Loading preview window for %s", s.Preview.BlobName),
		VisibleLines:     max(1, visibleLines),
		Cursor:           s.Preview.Cursor,
		KnownSize:        s.Preview.BlobSize,
		KnownContentType: s.Preview.ContentType,
	}
}

func (s *Session) ApplySubscriptionsResult(subscriptions []azure.Subscription, done bool, err error) {
	if err != nil {
		s.SetError("Failed to load subscriptions", err)
		return
	}
	s.ClearError()
	s.Subscriptions = subscriptions
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d subscriptions.", len(subscriptions))
	}
}

func (s *Session) ApplyAccountsResult(subscriptionID string, accounts []blob.Account, done bool, err error) bool {
	if !s.AcceptAccountsResult(subscriptionID) {
		return false
	}
	if err != nil {
		s.SetError("Failed to load storage accounts in "+subscriptionDisplayName(s.CurrentSubscription), err)
		return true
	}
	s.ClearError()
	s.Accounts = accounts
	if done {
		s.Loading = false
		s.Status = "Loaded " + itoa(len(accounts)) + " storage accounts from " + subscriptionDisplayName(s.CurrentSubscription) + "."
	}
	return true
}

func (s *Session) ApplyContainersResult(account blob.Account, containers []blob.ContainerInfo, done bool, err error) bool {
	if !s.AcceptContainersResult(account) {
		return false
	}
	if err != nil {
		s.SetError("Failed to load containers for "+account.Name, err)
		return true
	}
	s.ClearError()
	s.Containers = containers
	if done {
		s.Loading = false
		s.Status = "Loaded " + itoa(len(containers)) + " containers from " + account.Name + "."
	}
	return true
}

func (s *Session) ApplyBlobsResult(account blob.Account, container, prefix string, loadAll bool, query string, blobs []blob.BlobEntry, done bool, err error, hierarchyLimit int) bool {
	if !s.AcceptBlobsResult(account, container, prefix, loadAll, query) {
		return false
	}
	if err != nil {
		s.SetError("Failed to load blobs in "+account.Name+"/"+container, err)
		return true
	}
	s.ClearError()
	s.Blobs = blobs
	if done {
		s.Loading = false
		s.Status = blobLoadStatus(account, container, prefix, query, len(blobs), loadAll, hierarchyLimit)
	}
	return true
}

func (s *Session) ApplyDownloadResult(destinationRoot string, total, downloaded, failed int, failures []string, err error) {
	s.Loading = false
	if err != nil {
		s.LastErr = err.Error()
		s.Status = "Failed to download blobs"
		return
	}
	if failed > 0 {
		s.LastErr = strings.Join(failures, " | ")
		s.Status = "Downloaded " + itoa(downloaded) + "/" + itoa(total) + " blobs to " + destinationRoot
		return
	}
	s.ClearError()
	s.Status = "Downloaded " + itoa(downloaded) + " blob(s) to " + destinationRoot
}

func (s *Session) ResetPreview() {
	s.Preview = Preview{}
	s.SetPreviewOpen(false)
}

func (s *Session) BeginPreview(entry blob.BlobEntry) {
	s.SetPreviewOpen(true)
	s.Focus = PreviewPane
	s.Preview.BlobName = entry.Name
	s.Preview.BlobSize = entry.Size
	s.Preview.ContentType = entry.ContentType
	s.Preview.Binary = false
	s.Preview.Cursor = 0
	s.Preview.WindowStart = 0
	s.Preview.WindowData = nil
	s.Preview.LineStarts = nil
	s.Preview.RequestID++
}

func (s *Session) ApplyPreviewResult(account blob.Account, container, blobName string, requestID int, blobSize int64, contentType string, cursor int64, windowStart int64, data []byte, binary bool) bool {
	if !s.AcceptPreviewResult(account, container, blobName, requestID) {
		return false
	}
	s.ClearError()
	s.Loading = false
	s.Preview.BlobSize = blobSize
	if strings.TrimSpace(contentType) != "" {
		s.Preview.ContentType = contentType
	}
	s.Preview.Cursor = clampInt64(cursor, 0, maxInt64(0, blobSize-1))
	s.Preview.WindowStart = windowStart
	s.Preview.WindowData = data
	s.Preview.LineStarts = computeLineStarts(data)
	s.Preview.Binary = binary
	s.Status = fmt.Sprintf("Previewing %s", blobName)
	return true
}

func (s *Session) JumpPreviewToTop() {
	s.Preview.Cursor = 0
}

func (s *Session) JumpPreviewToBottom() {
	if s.Preview.BlobSize <= 0 {
		s.Preview.Cursor = 0
		return
	}
	s.Preview.Cursor = s.Preview.BlobSize - 1
}

func (s *Session) MovePreviewCursorByLines(delta int) bool {
	if !s.Preview.Open || delta == 0 {
		return false
	}
	if len(s.Preview.WindowData) == 0 || len(s.Preview.LineStarts) == 0 {
		return false
	}
	local := s.PreviewLocalLine()
	target := local + delta
	if target < 0 {
		target = 0
	}
	if target >= len(s.Preview.LineStarts) {
		target = len(s.Preview.LineStarts) - 1
	}
	s.Preview.Cursor = s.Preview.WindowStart + int64(s.Preview.LineStarts[target])
	if s.Preview.BlobSize > 0 {
		s.Preview.Cursor = clampInt64(s.Preview.Cursor, 0, s.Preview.BlobSize-1)
	}
	return true
}

func (s Session) PreviewLocalLine() int {
	if len(s.Preview.LineStarts) == 0 {
		return 0
	}
	localOffset := int(clampInt64(s.Preview.Cursor-s.Preview.WindowStart, 0, int64(len(s.Preview.WindowData))))
	idx := sort.Search(len(s.Preview.LineStarts), func(i int) bool {
		return s.Preview.LineStarts[i] > localOffset
	})
	if idx == 0 {
		return 0
	}
	line := idx - 1
	if line >= len(s.Preview.LineStarts) {
		return len(s.Preview.LineStarts) - 1
	}
	return line
}

func (s Session) PreviewNeedsWindowLoad(visibleLines int) bool {
	windowEnd := s.Preview.WindowStart + int64(len(s.Preview.WindowData))
	needLoad := len(s.Preview.WindowData) == 0 || s.Preview.Cursor < s.Preview.WindowStart || s.Preview.Cursor >= windowEnd
	if needLoad {
		return true
	}
	if len(s.Preview.LineStarts) == 0 {
		return false
	}
	visible := max(1, visibleLines)
	local := s.PreviewLocalLine()
	if s.Preview.WindowStart > 0 && local < visible*previewBufferViewports {
		return true
	}
	if windowEnd < s.Preview.BlobSize && local > len(s.Preview.LineStarts)-visible*(previewBufferViewports+1) {
		return true
	}
	return false
}

func (s *Session) SetPreviewOpen(open bool) {
	s.State.SetPreviewOpen(open)
	s.Preview.Open = open
}

func (s *Session) NextFocus()     { s.State.NextFocus() }
func (s *Session) PreviousFocus() { s.State.PreviousFocus() }

func (s *Session) NavigateLeft() LeftAction {
	if s.Focus == PreviewPane {
		s.SetPreviewOpen(false)
		s.Focus = BlobsPane
		return LeftFocusBlobs
	}
	return s.State.NavigateLeft()
}

func (s Session) AcceptAccountsResult(subscriptionID string) bool {
	return s.HasSubscription && s.CurrentSubscription.ID == subscriptionID
}

func (s Session) AcceptContainersResult(account blob.Account) bool {
	return s.HasAccount && SameAccount(s.CurrentAccount, account)
}

func (s Session) AcceptBlobsResult(account blob.Account, container, prefix string, loadAll bool, query string) bool {
	if !s.HasAccount || !s.HasContainer {
		return false
	}
	if !SameAccount(s.CurrentAccount, account) || s.ContainerName != container {
		return false
	}
	return s.Prefix == prefix && s.BlobLoadAll == loadAll && s.BlobSearchQuery == query
}

func (s Session) AcceptPreviewResult(account blob.Account, container, blobName string, requestID int) bool {
	if !s.Preview.Open || !s.HasAccount || !s.HasContainer {
		return false
	}
	if !SameAccount(s.CurrentAccount, account) || s.ContainerName != container {
		return false
	}
	if requestID != s.Preview.RequestID {
		return false
	}
	return blobName == s.Preview.BlobName
}

func (s *State) SelectSubscription(sub azure.Subscription) {
	s.CurrentSubscription = sub
	s.HasSubscription = true
	s.HasAccount = false
	s.HasContainer = false
	s.CurrentAccount = blob.Account{}
	s.ContainerName = ""
	s.Prefix = ""
	s.BlobLoadAll = false
	s.BlobSearchQuery = ""
	s.PreviewOpen = false
	s.Focus = AccountsPane
}

func (s *State) SelectAccount(account blob.Account) {
	s.CurrentAccount = account
	s.HasAccount = true
	s.HasContainer = false
	s.ContainerName = ""
	s.Prefix = ""
	s.BlobLoadAll = false
	s.BlobSearchQuery = ""
	s.PreviewOpen = false
	s.Focus = ContainersPane
}

func (s *State) SelectContainer(name string) {
	s.ContainerName = name
	s.HasContainer = true
	s.Prefix = ""
	s.BlobLoadAll = false
	s.BlobSearchQuery = ""
	s.PreviewOpen = false
	s.Focus = BlobsPane
}

func (s *State) OpenPrefix(prefix string) {
	s.Prefix = prefix
	s.BlobSearchQuery = ""
}

func (s *State) SetPreviewOpen(open bool) {
	s.PreviewOpen = open
	if !open && s.Focus == PreviewPane {
		s.Focus = BlobsPane
	}
}

func (s *State) NextFocus() {
	order := []Pane{SubscriptionsPane, AccountsPane, ContainersPane, BlobsPane}
	if s.PreviewOpen {
		order = append(order, PreviewPane)
	}
	idx := 0
	for i, pane := range order {
		if pane == s.Focus {
			idx = i
			break
		}
	}
	s.Focus = order[(idx+1)%len(order)]
}

func (s *State) PreviousFocus() {
	order := []Pane{SubscriptionsPane, AccountsPane, ContainersPane, BlobsPane}
	if s.PreviewOpen {
		order = append(order, PreviewPane)
	}
	idx := 0
	for i, pane := range order {
		if pane == s.Focus {
			idx = i
			break
		}
	}
	idx = (idx - 1 + len(order)) % len(order)
	s.Focus = order[idx]
}

func (s *State) NavigateLeft() LeftAction {
	switch s.Focus {
	case PreviewPane:
		s.Focus = BlobsPane
		return LeftFocusBlobs
	case BlobsPane:
		if s.HasContainer && !s.BlobLoadAll && s.Prefix != "" {
			s.Prefix = ParentPrefix(s.Prefix)
			s.BlobSearchQuery = ""
			return LeftHierarchyUp
		}
		s.Focus = ContainersPane
		return LeftFocusContainers
	case ContainersPane:
		s.Focus = AccountsPane
		return LeftFocusAccounts
	case AccountsPane:
		s.Focus = SubscriptionsPane
		return LeftFocusSubscriptions
	default:
		return LeftNoop
	}
}

func SameAccount(a, b blob.Account) bool {
	return a.Name == b.Name && a.SubscriptionID == b.SubscriptionID
}

func ParentPrefix(prefix string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	idx := strings.LastIndex(prefix, "/")
	if idx < 0 {
		return ""
	}
	return prefix[:idx+1]
}

func BlobSearchPrefix(currentPrefix, query string) string {
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

const previewBufferViewports = 10

func subscriptionDisplayName(sub azure.Subscription) string {
	name := strings.TrimSpace(sub.Name)
	if name != "" {
		return name
	}
	return sub.ID
}

func blobLoadStatus(account blob.Account, container, prefix, query string, count int, loadAll bool, hierarchyLimit int) string {
	if loadAll {
		return "Loaded all " + itoa(count) + " blobs in " + account.Name + "/" + container
	}
	if query != "" {
		effectivePrefix := BlobSearchPrefix(prefix, query)
		return "Found " + itoa(count) + " blobs by prefix \"" + effectivePrefix + "\" in " + account.Name + "/" + container
	}
	return "Loaded " + itoa(count) + " entries (max " + itoa(hierarchyLimit) + ") in " + account.Name + "/" + container + " under \"" + prefix + "\""
}

func itoa(v int) string { return strconv.Itoa(v) }

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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
