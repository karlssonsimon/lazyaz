package blobapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// copyTargets builds the copy palette entries for the current scope.
// Values are the real untruncated strings — notably the prefix path,
// which the header breadcrumb renders truncated.
func (m Model) copyTargets() []ui.CopyTarget {
	var targets []ui.CopyTarget

	if item, ok := m.blobsList.SelectedItem().(blobItem); ok {
		if item.blob.IsPrefix {
			targets = append(targets, ui.CopyTarget{Label: "Directory path", Value: item.blob.Name})
		} else {
			targets = append(targets, ui.CopyTarget{Label: "Blob name", Value: item.blob.Name})
			if m.hasContainer && m.currentAccount.BlobEndpoint != "" {
				targets = append(targets, ui.CopyTarget{
					Label: "Blob URL",
					Value: blobURL(m.currentAccount.BlobEndpoint, m.containerName, item.blob.Name),
				})
			}
		}
	}
	if m.prefix != "" {
		targets = append(targets, ui.CopyTarget{Label: "Prefix path", Value: m.prefix})
	}
	if m.hasContainer {
		targets = append(targets, ui.CopyTarget{Label: "Container", Value: m.containerName})
	}
	if m.hasAccount {
		targets = append(targets, ui.CopyTarget{Label: "Account", Value: m.currentAccount.Name})
		if m.currentAccount.BlobEndpoint != "" {
			targets = append(targets, ui.CopyTarget{Label: "Blob endpoint", Value: m.currentAccount.BlobEndpoint})
		}
	}
	if len(m.markedBlobs) > 0 {
		targets = append(targets, ui.CopyTarget{
			Label: fmt.Sprintf("Marked names (%d)", len(m.markedBlobs)),
			Value: strings.Join(m.sortedMarkedBlobNames(), "\n"),
		})
	}
	return targets
}

// blobURL joins endpoint, container, and blob name into the public
// blob URL, tolerating a trailing slash on the endpoint.
func blobURL(endpoint, container, name string) string {
	return strings.TrimSuffix(endpoint, "/") + "/" + container + "/" + name
}

// openCopyPalette opens the palette, or explains why there's nothing
// to copy yet.
func (m Model) openCopyPalette() (Model, tea.Cmd) {
	targets := m.copyTargets()
	if len(targets) == 0 {
		m.Notify(appshell.LevelInfo, "Nothing to copy here yet")
		return m, nil
	}
	m.copyOverlay.Open(targets)
	return m, nil
}
