package core

import (
	"fmt"
	"strings"
	"time"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
)

type Action string

const (
	ActionRefresh            Action = "blob.refresh"
	ActionFocusNext          Action = "blob.focus.next"
	ActionFocusPrevious      Action = "blob.focus.previous"
	ActionNavigateLeft       Action = "blob.navigate.left"
	ActionSelectSubscription Action = "blob.select.subscription"
	ActionSelectAccount      Action = "blob.select.account"
	ActionSelectContainer    Action = "blob.select.container"
	ActionOpenBlob           Action = "blob.open"
	ActionToggleLoadAll      Action = "blob.toggle_load_all"
	ActionApplySearch        Action = "blob.apply_search"
	ActionToggleMark         Action = "blob.toggle_mark"
	ActionToggleVisual       Action = "blob.toggle_visual"
	ActionExitVisual         Action = "blob.exit_visual"
	ActionDownload           Action = "blob.download"
	ActionPreviewTop         Action = "blob.preview.top"
	ActionPreviewBottom      Action = "blob.preview.bottom"
	ActionPreviewMoveLines   Action = "blob.preview.move_lines"
	ActionPreviewClose       Action = "blob.preview.close"
)

type ActionRequest struct {
	Action            Action
	Subscription      azure.Subscription
	Account           blob.Account
	ContainerName     string
	Blob              blob.BlobEntry
	DisplayName       string
	CurrentBlobName   string
	SelectionCount    int
	LineDelta         int
	VisibleLines      int
	SearchQuery       string
	BlobNames         []string
	DestinationRoot   string
	HierarchyLimit    int
	PrefixSearchLimit int
}

type ActionResult struct {
	LoadRequest LoadRequest
	OpenPreview bool
	Status      string
	LeftAction  LeftAction
}

type Service struct{ session *Session }

func NewService(session *Session) *Service {
	if session == nil {
		s := NewSession()
		session = &s
	}
	return &Service{session: session}
}

func (s *Service) Session() *Session { return s.session }

func (s *Service) Dispatch(req ActionRequest) (ActionResult, error) {
	session := s.session
	if session == nil {
		return ActionResult{}, fmt.Errorf("blob service has no session")
	}
	visibleLines := max(1, req.VisibleLines)
	switch req.Action {
	case ActionRefresh:
		return ActionResult{LoadRequest: session.RefreshRequest(req.HierarchyLimit, req.PrefixSearchLimit)}, nil
	case ActionFocusNext:
		session.NextFocus()
		return ActionResult{}, nil
	case ActionFocusPrevious:
		session.PreviousFocus()
		return ActionResult{}, nil
	case ActionNavigateLeft:
		leftAction, load := session.NavigateLeftRequest(req.HierarchyLimit)
		return ActionResult{LoadRequest: load, LeftAction: leftAction}, nil
	case ActionSelectSubscription:
		return ActionResult{LoadRequest: session.SelectSubscriptionRequest(req.Subscription)}, nil
	case ActionSelectAccount:
		return ActionResult{LoadRequest: session.SelectAccountRequest(req.Account)}, nil
	case ActionSelectContainer:
		return ActionResult{LoadRequest: session.SelectContainerRequest(req.ContainerName, req.HierarchyLimit)}, nil
	case ActionOpenBlob:
		result := session.OpenBlobEntry(req.Blob, req.HierarchyLimit, visibleLines)
		return ActionResult{LoadRequest: result.LoadRequest, OpenPreview: result.OpenPreview, Status: result.Status}, nil
	case ActionToggleLoadAll:
		return ActionResult{LoadRequest: session.ToggleLoadAllRequest(req.HierarchyLimit)}, nil
	case ActionApplySearch:
		query := strings.TrimSpace(req.SearchQuery)
		session.BlobSearchQuery = query
		if query == "" {
			return ActionResult{LoadRequest: LoadRequest{Kind: LoadHierarchyBlobs, Account: session.CurrentAccount, ContainerName: session.ContainerName, Prefix: session.Prefix, Limit: req.HierarchyLimit, Status: fmt.Sprintf("Loading up to %d entries under %q", req.HierarchyLimit, session.Prefix)}}, nil
		}
		return ActionResult{LoadRequest: LoadRequest{Kind: LoadSearchBlobs, Account: session.CurrentAccount, ContainerName: session.ContainerName, Prefix: session.Prefix, Query: query, Limit: req.PrefixSearchLimit, Status: fmt.Sprintf("Searching blobs by prefix %q...", BlobSearchPrefix(session.Prefix, query))}}, nil
	case ActionToggleMark:
		return ActionResult{Status: session.ToggleMarkedBlob(req.Blob, req.DisplayName)}, nil
	case ActionToggleVisual:
		return ActionResult{Status: session.ToggleVisualLineMode(req.CurrentBlobName, req.SelectionCount)}, nil
	case ActionExitVisual:
		return ActionResult{Status: session.ExitVisualMode()}, nil
	case ActionPreviewTop:
		session.JumpPreviewToTop()
		if session.PreviewNeedsWindowLoad(visibleLines) {
			return ActionResult{LoadRequest: session.PreviewWindowRequest(visibleLines)}, nil
		}
		return ActionResult{}, nil
	case ActionPreviewBottom:
		session.JumpPreviewToBottom()
		if session.PreviewNeedsWindowLoad(visibleLines) {
			return ActionResult{LoadRequest: session.PreviewWindowRequest(visibleLines)}, nil
		}
		return ActionResult{}, nil
	case ActionPreviewMoveLines:
		session.MovePreviewCursorByLines(req.LineDelta)
		if session.PreviewNeedsWindowLoad(visibleLines) {
			return ActionResult{LoadRequest: session.PreviewWindowRequest(visibleLines)}, nil
		}
		return ActionResult{}, nil
	case ActionPreviewClose:
		session.SetPreviewOpen(false)
		session.Focus = BlobsPane
		return ActionResult{}, nil
	case ActionDownload:
		return ActionResult{LoadRequest: LoadRequest{Kind: LoadDownload, Account: session.CurrentAccount, ContainerName: session.ContainerName, BlobNames: append([]string(nil), req.BlobNames...), DestinationRoot: req.DestinationRoot, Status: fmt.Sprintf("Downloading %d blob(s) to %s", len(req.BlobNames), req.DestinationRoot)}}, nil
	default:
		return ActionResult{}, fmt.Errorf("unsupported blob action %q", req.Action)
	}
}

type Snapshot struct {
	Focus               string              `json:"focus"`
	HasSubscription     bool                `json:"has_subscription"`
	CurrentSubscription *SubscriptionState  `json:"current_subscription,omitempty"`
	HasAccount          bool                `json:"has_account"`
	CurrentAccount      *AccountState       `json:"current_account,omitempty"`
	HasContainer        bool                `json:"has_container"`
	ContainerName       string              `json:"container_name"`
	Prefix              string              `json:"prefix"`
	BlobLoadAll         bool                `json:"blob_load_all"`
	BlobSearchQuery     string              `json:"blob_search_query"`
	Subscriptions       []SubscriptionState `json:"subscriptions"`
	Accounts            []AccountState      `json:"accounts"`
	Containers          []ContainerState    `json:"containers"`
	Blobs               []BlobState         `json:"blobs"`
	MarkedBlobNames     []string            `json:"marked_blob_names"`
	VisualLineMode      bool                `json:"visual_line_mode"`
	VisualAnchor        string              `json:"visual_anchor"`
	Preview             PreviewState        `json:"preview"`
	Loading             bool                `json:"loading"`
	Status              string              `json:"status"`
	LastErr             string              `json:"last_err"`
}

type SubscriptionState struct{ ID, Name, State string }
type AccountState struct{ Name, SubscriptionID, ResourceGroup, BlobEndpoint string }
type ContainerState struct{ Name string }
type BlobState struct {
	Name         string `json:"name"`
	IsPrefix     bool   `json:"is_prefix"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
	LastModified string `json:"last_modified"`
	AccessTier   string `json:"access_tier"`
}
type PreviewState struct {
	Open        bool   `json:"open"`
	BlobName    string `json:"blob_name"`
	BlobSize    int64  `json:"blob_size"`
	ContentType string `json:"content_type"`
	Binary      bool   `json:"binary"`
	Cursor      int64  `json:"cursor"`
	WindowStart int64  `json:"window_start"`
	RequestID   int    `json:"request_id"`
	WindowText  string `json:"window_text,omitempty"`
}

func (s *Service) Snapshot() Snapshot {
	session := s.session
	out := Snapshot{
		Focus: paneName(session.Focus), HasSubscription: session.HasSubscription, HasAccount: session.HasAccount, HasContainer: session.HasContainer,
		ContainerName: session.ContainerName, Prefix: session.Prefix, BlobLoadAll: session.BlobLoadAll, BlobSearchQuery: session.BlobSearchQuery,
		MarkedBlobNames: session.SortedMarkedBlobNames(), VisualLineMode: session.VisualLineMode, VisualAnchor: session.VisualAnchor,
		Loading: session.Loading, Status: session.Status, LastErr: session.LastErr,
		Preview: PreviewState{Open: session.Preview.Open, BlobName: session.Preview.BlobName, BlobSize: session.Preview.BlobSize, ContentType: session.Preview.ContentType, Binary: session.Preview.Binary, Cursor: session.Preview.Cursor, WindowStart: session.Preview.WindowStart, RequestID: session.Preview.RequestID, WindowText: string(session.Preview.WindowData)},
	}
	if session.HasSubscription {
		out.CurrentSubscription = &SubscriptionState{ID: session.CurrentSubscription.ID, Name: session.CurrentSubscription.Name, State: session.CurrentSubscription.State}
	}
	if session.HasAccount {
		out.CurrentAccount = &AccountState{Name: session.CurrentAccount.Name, SubscriptionID: session.CurrentAccount.SubscriptionID, ResourceGroup: session.CurrentAccount.ResourceGroup, BlobEndpoint: session.CurrentAccount.BlobEndpoint}
	}
	for _, sub := range session.Subscriptions {
		out.Subscriptions = append(out.Subscriptions, SubscriptionState{ID: sub.ID, Name: sub.Name, State: sub.State})
	}
	for _, a := range session.Accounts {
		out.Accounts = append(out.Accounts, AccountState{Name: a.Name, SubscriptionID: a.SubscriptionID, ResourceGroup: a.ResourceGroup, BlobEndpoint: a.BlobEndpoint})
	}
	for _, c := range session.Containers {
		out.Containers = append(out.Containers, ContainerState{Name: c.Name})
	}
	for _, b := range session.Blobs {
		out.Blobs = append(out.Blobs, BlobState{Name: b.Name, IsPrefix: b.IsPrefix, Size: b.Size, ContentType: b.ContentType, LastModified: b.LastModified.Format(time.RFC3339), AccessTier: b.AccessTier})
	}
	return out
}

func paneName(pane Pane) string {
	switch pane {
	case SubscriptionsPane:
		return "subscriptions"
	case AccountsPane:
		return "accounts"
	case ContainersPane:
		return "containers"
	case BlobsPane:
		return "blobs"
	case PreviewPane:
		return "preview"
	default:
		return "subscriptions"
	}
}
