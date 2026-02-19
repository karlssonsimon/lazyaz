# Azure Blob Explorer TUI (Go)

Small Azure Storage Explorer variant focused on Blob Storage, built with Bubble Tea.

## Features

- Azure authentication via `DefaultAzureCredential` (works with `az login`)
- Subscription-first navigation: select a subscription, then browse its storage accounts
- Multi-subscription and multi-account browsing
- Container listing
- Blob browsing with virtual folder navigation (`/` delimiter)
- Blobs load only after opening a container (no auto-open on account switch)
- Live fuzzy filtering in all panes (accounts, containers, blobs) with `/`
- Blob multi-selection in blob pane: `space` toggles mark, `v` enters visual line marking mode
- Download marked blobs with `D` to `downloads/<account>/<container>/...`
- Auth mode similar to Storage Explorer: Azure AD first, Shared Key fallback via ARM `ListKeys`
- Blob details in status bar (size, modified time, content type, tier, metadata count)

## Requirements

- Go 1.22+
- Azure CLI installed and logged in: `az login`
- RBAC permissions on target accounts (for example `Storage Blob Data Reader`)

## Run

```bash
go run ./cmd/azblob-tui
```

## Keys

- `tab` / `shift+tab`: move focus between panes
- `enter` / `l` / `right`: open selected item and move focus right
- `h` / `left`: move focus left, or go up folder when browsing blobs
- `ctrl+d` / `ctrl+u`: scroll half-page down/up in the focused pane
- `backspace`: go to parent folder in blobs
- `/`: filter the focused pane live (fzf-style), `enter` applies and exits filter input
- `space`: toggle mark on current blob
- `v` / `V`: visual line mode in blobs (move to mark multiple)
- `D`: download all marked blobs (or current blob if none marked)
- `r`: refresh current view
- `d`: reload subscriptions
- `q`: quit

## Notes

- This tool is Blob-only for now.
- Filtering is local to the currently loaded list in each pane.
- Discovery uses ARM list APIs (subscriptions -> storage accounts), while browsing uses Blob data-plane APIs.
- Shared Key fallback requires permission to list storage account keys and that Shared Key access is allowed on the account.
- Marked selection is container-scoped and intentionally action-oriented so more bulk actions can be added later.
