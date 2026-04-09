package keymap

// Keymap holds all action bindings for the application. Every field has a
// json tag matching the key name in the JSON keymap file. Fields are grouped
// by scope but the JSON file is flat.
type Keymap struct {
	// Shared — used by all apps.
	Quit                Binding `json:"quit"`
	Cancel              Binding `json:"cancel"`
	HalfPageDown        Binding `json:"half_page_down"`
	HalfPageUp          Binding `json:"half_page_up"`
	NextFocus           Binding `json:"next_focus"`
	PreviousFocus       Binding `json:"previous_focus"`
	ReloadSubscriptions Binding `json:"reload_subscriptions"`
	RefreshScope        Binding `json:"refresh_scope"`
	FilterInput         Binding `json:"filter_input"`
	OpenFocused         Binding `json:"open_focused"`
	OpenFocusedAlt      Binding `json:"open_focused_alt"`
	NavigateLeft        Binding `json:"navigate_left"`
	BackspaceUp         Binding `json:"backspace_up"`
	SubscriptionPicker  Binding `json:"subscription_picker"`
	ToggleThemePicker   Binding `json:"toggle_theme_picker"`
	ToggleHelp          Binding `json:"toggle_help"`
	ToggleNotifications Binding `json:"toggle_notifications"`
	ToggleStreams       Binding `json:"toggle_streams"`

	// Overlay navigation — reused by theme, subscription, tab pickers.
	ThemeUp     Binding `json:"theme_up"`
	ThemeDown   Binding `json:"theme_down"`
	ThemeApply  Binding `json:"theme_apply"`
	ThemeCancel Binding `json:"theme_cancel"`

	// Tab management — tabbed app only.
	NewTab         Binding `json:"new_tab"`
	CloseTab       Binding `json:"close_tab"`
	NextTab        Binding `json:"next_tab"`
	PrevTab        Binding `json:"prev_tab"`
	CommandPalette Binding `json:"command_palette"`
	Jump1          Binding `json:"jump_1"`
	Jump2          Binding `json:"jump_2"`
	Jump3          Binding `json:"jump_3"`
	Jump4          Binding `json:"jump_4"`
	Jump5          Binding `json:"jump_5"`
	Jump6          Binding `json:"jump_6"`
	Jump7          Binding `json:"jump_7"`
	Jump8          Binding `json:"jump_8"`
	Jump9          Binding `json:"jump_9"`

	// Blob app.
	ToggleLoadAll     Binding `json:"toggle_load_all"`
	ToggleMark        Binding `json:"toggle_mark"`
	ToggleVisualLine  Binding `json:"toggle_visual_line"`
	ExitVisualLine    Binding `json:"exit_visual_line"`
	VisualSwapAnchor  Binding `json:"visual_swap_anchor"`
	DownloadSelection Binding `json:"download_selection"`
	SortBlobs         Binding `json:"sort_blobs"`
	BlobVisualMove    Binding `json:"blob_visual_move"`

	// Blob preview.
	PreviewBack          Binding `json:"preview_back"`
	PreviewNextFocus     Binding `json:"preview_next_focus"`
	PreviewPreviousFocus Binding `json:"preview_previous_focus"`
	PreviewDown          Binding `json:"preview_down"`
	PreviewUp            Binding `json:"preview_up"`
	PreviewBottom        Binding `json:"preview_bottom"`
	PreviewTopPrefix     Binding `json:"preview_top_prefix"`

	// Service Bus app.
	ShowActiveQueue     Binding `json:"show_active_queue"`
	ShowDeadLetterQueue Binding `json:"show_dead_letter_queue"`
	RequeueDLQ          Binding `json:"requeue_dlq"`
	DeleteDuplicate     Binding `json:"delete_duplicate"`
	ToggleDLQFilter     Binding `json:"toggle_dlq_filter"`
	MessageBack         Binding `json:"message_back"`

	// Key Vault app.
	YankSecret Binding `json:"yank_secret"`

	// Shared — inspect selected item.
	Inspect Binding `json:"inspect"`
}

// JumpIndex returns the tab index (0-based) if key matches any Jump binding.
func (k Keymap) JumpIndex(key string) (int, bool) {
	jumps := []Binding{k.Jump1, k.Jump2, k.Jump3, k.Jump4, k.Jump5, k.Jump6, k.Jump7, k.Jump8, k.Jump9}
	for i, b := range jumps {
		if b.Matches(key) {
			return i, true
		}
	}
	return 0, false
}
