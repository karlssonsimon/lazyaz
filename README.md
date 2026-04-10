# lazyaz

A keyboard-driven TUI for Azure — Blob Storage, Service Bus, and Key Vault — built with Bubble Tea.

`lazyaz` is to `az` what `lazygit` is to `git`: a fast, fully-keyboard explorer that turns common Azure browse/inspect tasks into a few keystrokes.

## Install

```bash
go install github.com/karlssonsimon/lazyaz/cmd/lazyaz@latest
```

## Features

- Azure authentication via `DefaultAzureCredential` (works with `az login`)
- Subscription-first navigation: select a subscription, then browse its storage accounts
- Multi-subscription and multi-account browsing
- Container listing
- Blob browsing with virtual folder navigation (`/` delimiter)
- Blob load modes: hierarchical browse by default, optional full container load on demand
- Blobs load a capped initial subset on container open; use prefix search to narrow or `a` for full load
- Live fuzzy filtering in subscriptions/accounts/containers and in blobs when full-load mode is active
- Prefix search in blobs pane when not in full-load mode (press `/`, type prefix, `enter`; server-side, limited)
- Blob multi-selection in blob pane: `space` toggles mark, `v` enters visual line marking mode
- Download marked blobs with `D` to `downloads/<account>/<container>/...`
- In-pane blob preview opened from blob entries (`enter`/`l`) with streamed range reads and syntax highlighting for JSON/XML/CSV
- Auth mode similar to Storage Explorer: Azure AD first, Shared Key fallback via ARM `ListKeys`
- Blob details in status bar (size, modified time, content type, tier, metadata count)

## Requirements

- Go 1.22+
- Azure CLI installed and logged in: `az login`
- RBAC permissions on target accounts (for example `Storage Blob Data Reader`)

## Run

```bash
go run ./cmd/lazyaz
```

## Keys

- `tab` / `shift+tab`: move focus between panes
- `enter` / `l` / `right`: open selected item and move focus right
- `h` / `left`: move focus left, or go up folder when browsing blobs
- `ctrl+d` / `ctrl+u`: scroll half-page down/up in the focused pane
- In preview pane: `j`/`k` line scroll, `ctrl+d`/`ctrl+u` half-page, `gg` top, `G` bottom
- `backspace`: go to parent folder in blobs
- `/`: filter the focused pane live (fzf-style), `enter` applies and exits filter input
- `a`: toggle blob load-all mode for current container
- `space`: toggle mark on current blob
- `v` / `V`: visual line mode in blobs (move to mark multiple)
- `D`: download all marked blobs (or current blob if none marked)
- `r`: refresh current view
- `d`: reload subscriptions
- `q`: quit

## Notes

- Filtering is local to the currently loaded list in each pane.
- In blobs pane: default mode uses server-side prefix search; full-load mode uses local fuzzy filtering.
- Preview uses blob range reads with buffered windows to avoid loading full files when scrolling.
- Discovery uses ARM list APIs (subscriptions -> storage accounts), while browsing uses Blob data-plane APIs.
- Shared Key fallback requires permission to list storage account keys and that Shared Key access is allowed on the account.
- Marked selection is container-scoped and intentionally action-oriented so more bulk actions can be added later.
