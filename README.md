# lazyaz

A keyboard-driven TUI for Azure — Blob Storage, Service Bus, and Key Vault.

`lazyaz` is to `az` what `lazygit` is to `git`: a fast terminal explorer that turns
common Azure browse-and-inspect tasks into a few keystrokes. No more clicking
through the Azure Portal to peek at a dead-letter queue or download a blob.

<!-- TODO: add a GIF/screenshot here -->

## Install

```bash
go install github.com/karlssonsimon/lazyaz/cmd/lazyaz@latest
```

Requires Go 1.22+ and Azure CLI logged in (`az login`).

## Quick start

```bash
az login
lazyaz
```

Pick a subscription and start browsing. Press `?` at any time to see keybindings
for the current view.

## What it does

**Blob Storage** — Browse containers, navigate virtual folders, preview files
with syntax highlighting, multi-select and download in bulk.

**Service Bus** — Peek active and dead-letter queues, requeue or move DLQ
messages, inspect topics and subscriptions.

**Key Vault** — Browse vaults and secrets, view version history, yank secret
values to clipboard.

All three run side by side in tabs with per-tab subscription selection.

## Navigation

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Cycle pane focus |
| `Enter` / `l` | Open / drill in |
| `h` / `Left` | Go back |
| `/` | Filter focused pane |
| `Ctrl+D` / `Ctrl+U` | Half-page scroll |
| `S` | Switch subscription |
| `a` | Action menu |
| `K` | Toggle inspect panel |
| `?` | Help |
| `Ctrl+P` | Command palette |
| `Ctrl+T` | New tab |

## Configuration

Config lives in `~/.config/lazyaz/`:

- `config.yaml` — theme, download directory, startup tabs, keymap
- `themes/` — Base16 color schemes (switch at runtime with `T`)
- `keymaps/` — JSON keybinding overrides

## Documentation

Full documentation: **[karlssonsimon.github.io/lazyaz](https://karlssonsimon.github.io/lazyaz)**

## Thanks

Thanks to [bosvik](https://github.com/bosvik) and [svenclaesson](https://github.com/svenclaesson) for contributions and feedback.

## License

MIT — see [`LICENSE`](LICENSE). Third-party licenses in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
