package blobapp

import (
	"context"
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	tea "charm.land/bubbletea/v2"
)

// crudDoneMsg is emitted when a CRUD command finishes. Carries the
// user-facing summary line and the level so Update can Notify + refresh.
type crudDoneMsg struct {
	level   appshell.NotificationLevel
	message string
}

// deleteBlobCmd deletes a single blob.
func deleteBlobCmd(svc *blob.Service, account blob.Account, containerName, blobName string) tea.Cmd {
	return func() tea.Msg {
		err := svc.DeleteBlob(context.Background(), account, containerName, blobName)
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
		results, err := svc.DeleteBlobs(context.Background(), account, containerName, names)
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
		err := svc.RenameBlob(context.Background(), account, containerName, oldName, newName)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Rename failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Renamed %s → %s", oldName, newName)}
	}
}

func createContainerCmd(svc *blob.Service, account blob.Account, containerName string) tea.Cmd {
	return func() tea.Msg {
		err := svc.CreateContainer(context.Background(), account, containerName)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Create container failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Created container %s", containerName)}
	}
}

func deleteContainerCmd(svc *blob.Service, account blob.Account, containerName string) tea.Cmd {
	return func() tea.Msg {
		err := svc.DeleteContainer(context.Background(), account, containerName)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete container failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted container %s", containerName)}
	}
}

func createDirectoryCmd(svc *blob.Service, account blob.Account, containerName, directoryPath string) tea.Cmd {
	return func() tea.Msg {
		err := svc.CreateDirectory(context.Background(), account, containerName, directoryPath)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Create folder failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Created folder %s", directoryPath)}
	}
}

func deleteDirectoryCmd(svc *blob.Service, account blob.Account, containerName, directoryPath string) tea.Cmd {
	return func() tea.Msg {
		err := svc.DeleteDirectory(context.Background(), account, containerName, directoryPath)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Delete folder failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Deleted folder %s", directoryPath)}
	}
}

func renameDirectoryCmd(svc *blob.Service, account blob.Account, containerName, oldPath, newPath string) tea.Cmd {
	return func() tea.Msg {
		err := svc.RenameDirectory(context.Background(), account, containerName, oldPath, newPath)
		if err != nil {
			return crudDoneMsg{level: appshell.LevelError, message: fmt.Sprintf("Rename folder failed: %v", err)}
		}
		return crudDoneMsg{level: appshell.LevelSuccess, message: fmt.Sprintf("Renamed folder %s → %s", oldPath, newPath)}
	}
}
