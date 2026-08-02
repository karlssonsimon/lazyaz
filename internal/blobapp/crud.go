package blobapp

import (
	"context"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	tea "charm.land/bubbletea/v2"
)

// crudTimeout bounds a single blob or container mutation. Without a
// deadline a stalled connection leaves the command goroutine parked
// forever and the pane's spinner never resolves.
const crudTimeout = 30 * time.Second

// perBlobDeleteBudget scales the bulk-delete deadline with the size of
// the batch, since DeleteBlobs walks the names sequentially. A flat
// timeout would abort a large but healthy selection midway.
const perBlobDeleteBudget = 10 * time.Second

// longCrudTimeout covers the mutations whose duration scales with the
// data rather than being a single API call: renaming a blob on a
// flat-namespace account polls an async server-side copy, and deleting a
// virtual folder there enumerates and deletes every blob under the
// prefix one by one.
const longCrudTimeout = 15 * time.Minute

// crudDoneMsg is emitted when a CRUD command finishes. Carries the
// user-facing summary line and the level so Update can Notify + refresh.
type crudDoneMsg struct {
	level   appshell.NotificationLevel
	message string
}

// deleteBlobCmd deletes a single blob.
func deleteBlobCmd(svc *blob.Service, account blob.Account, containerName, blobName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), crudTimeout)
		defer cancel()
		err := svc.DeleteBlob(ctx, account, containerName, blobName)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete %s failed: %v", blobName, err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted %s", blobName)}
	}
}

// deleteMarkedBlobsCmd deletes every blob name in names. Reports the
// per-blob breakdown as a single summary line.
func deleteMarkedBlobsCmd(svc *blob.Service, account blob.Account, containerName string, names []string) tea.Cmd {
	return func() tea.Msg {
		timeout := time.Duration(len(names)) * perBlobDeleteBudget
		if timeout < crudTimeout {
			timeout = crudTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		results, err := svc.DeleteBlobs(ctx, account, containerName, names)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete aborted: %v", err)}
		}
		var failed []string
		for _, r := range results {
			if r.Err != nil {
				failed = append(failed, r.BlobName)
			}
		}
		if len(failed) == 0 {
			return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted %d blobs", len(results))}
		}
		return crudDoneMsg{
			level:   appshell.LevelWarn,
			message: fmt.Sprintf("Deleted %d of %d · failed: %v", len(results)-len(failed), len(results), failed),
		}
	}
}

func renameBlobCmd(svc *blob.Service, account blob.Account, containerName, oldName, newName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), longCrudTimeout)
		defer cancel()
		err := svc.RenameBlob(ctx, account, containerName, oldName, newName)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Rename failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Renamed %s → %s", oldName, newName)}
	}
}

func createContainerCmd(svc *blob.Service, account blob.Account, containerName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), crudTimeout)
		defer cancel()
		err := svc.CreateContainer(ctx, account, containerName)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Create container failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Created container %s", containerName)}
	}
}

func deleteContainerCmd(svc *blob.Service, account blob.Account, containerName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), crudTimeout)
		defer cancel()
		err := svc.DeleteContainer(ctx, account, containerName)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete container failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted container %s", containerName)}
	}
}

func createDirectoryCmd(svc *blob.Service, account blob.Account, containerName, directoryPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), crudTimeout)
		defer cancel()
		err := svc.CreateDirectory(ctx, account, containerName, directoryPath)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Create folder failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Created folder %s", directoryPath)}
	}
}

func deleteDirectoryCmd(svc *blob.Service, account blob.Account, containerName, directoryPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), longCrudTimeout)
		defer cancel()
		err := svc.DeleteDirectory(ctx, account, containerName, directoryPath)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete folder failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted folder %s", directoryPath)}
	}
}

func renameDirectoryCmd(svc *blob.Service, account blob.Account, containerName, oldPath, newPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), crudTimeout)
		defer cancel()
		err := svc.RenameDirectory(ctx, account, containerName, oldPath, newPath)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Rename folder failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Renamed folder %s → %s", oldPath, newPath)}
	}
}
