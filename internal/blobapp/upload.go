package blobapp

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/activity"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
)

// fileToUpload is one entry in the expanded upload batch: the local
// file path, the target blob name, and the file size.
type fileToUpload struct {
	localPath string
	blobName  string
	size      int64
}

// normalizeDestPrefix strips trailing slashes and converts backslashes
// to forward slashes. Empty string means "upload to container root".
func normalizeDestPrefix(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimRight(p, "/")
	return p
}

// buildBlobName joins destPrefix with relPath (which may itself contain
// subpath components). The result always uses forward-slash separators.
func buildBlobName(destPrefix, relPath string) string {
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	relPath = strings.TrimPrefix(relPath, "/")
	pref := normalizeDestPrefix(destPrefix)
	if pref == "" {
		return relPath
	}
	return pref + "/" + relPath
}

// uploadPlan is the deterministic plan of files to upload for a given
// selection. Built up front so we can compute total bytes and run the
// pre-flight existence check.
type uploadPlan struct {
	files      []fileToUpload
	totalBytes int64
}

// Sort returns the plan with files sorted by blob name (stable). Keeps
// progress output deterministic.
func (p uploadPlan) Sort() uploadPlan {
	sort.SliceStable(p.files, func(i, j int) bool {
		return p.files[i].blobName < p.files[j].blobName
	})
	return p
}

// blobNames returns the blob names in plan order. Used for the
// pre-flight existence check.
func (p uploadPlan) blobNames() []string {
	out := make([]string, len(p.files))
	for i, f := range p.files {
		out[i] = f.blobName
	}
	return out
}

// uploadWalker abstracts os.Stat + filepath.WalkDir so tests can feed
// synthetic trees.
type uploadWalker interface {
	Stat(path string) (isDir bool, size int64, err error)
	Walk(root string, fn func(path string, isDir bool, size int64, err error) error) error
}

// planUpload builds an uploadPlan from the selected paths and
// destination prefix. Directories are walked recursively; top-level
// files are placed as `<destPrefix>/<basename>`; files inside walked
// folders become `<destPrefix>/<rel-path-from-selected-root>`.
// Symlinks are NOT followed (security + cycle avoidance). Empty
// directories contribute no files (blob storage has no directory concept).
func planUpload(walker uploadWalker, selected []string, destPrefix string) (uploadPlan, error) {
	var plan uploadPlan
	for _, root := range selected {
		isDir, size, err := walker.Stat(root)
		if err != nil {
			return plan, err
		}
		if !isDir {
			plan.files = append(plan.files, fileToUpload{
				localPath: root,
				blobName:  buildBlobName(destPrefix, filepath.Base(root)),
				size:      size,
			})
			plan.totalBytes += size
			continue
		}
		// Directory: walk it. Blob names rooted at the directory's basename
		// so the uploaded tree mirrors the local structure.
		rootBase := filepath.Base(root)
		if err := walker.Walk(root, func(path string, isDir bool, size int64, err error) error {
			if err != nil {
				return err
			}
			if isDir {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			plan.files = append(plan.files, fileToUpload{
				localPath: path,
				blobName:  buildBlobName(destPrefix, filepath.Join(rootBase, rel)),
				size:      size,
			})
			plan.totalBytes += size
			return nil
		}); err != nil {
			return plan, err
		}
	}
	return plan.Sort(), nil
}

// uploader is the subset of the service the upload worker needs. Lets
// tests inject a fake without touching the Azure SDK.
type uploader interface {
	ExistingBlobs(ctx context.Context, blobNames []string) (map[string]struct{}, error)
	UploadBlob(ctx context.Context, blobName, localPath string, progress func(int64)) error
}

// runUpload starts the worker goroutine. Messages are pushed into the
// given msgs channel; runUpload itself returns immediately.
//
// The worker sequence:
//  1. Call ExistingBlobs for the pre-flight conflict check.
//  2. Push uploadStartedMsg.
//  3. For each file: prompt on conflict, upload (respecting policy), push progress.
//  4. Push uploadDoneMsg.
//  5. Close the channel.
//
// Each message includes a `next` tea.Cmd so the model's handler can
// chain the receive loop (same pattern as cache.Broker.recv).
func runUpload(ctx context.Context, up uploader, plan uploadPlan, destPrefix string, cancel context.CancelFunc, msgs chan<- tea.Msg) {
	go func() {
		defer close(msgs)

		conflicts, _ := up.ExistingBlobs(ctx, plan.blobNames())
		msgs <- uploadStartedMsg{
			totalBytes: plan.totalBytes,
			fileCount:  len(plan.files),
			conflicts:  conflicts,
		}

		var result uploadDoneMsg
		result.totalBytes = plan.totalBytes
		result.destPrefix = destPrefix

		policy := conflictSkip // default; overridden once user picks an "all" answer
		explicitPolicy := false

		for i, f := range plan.files {
			if ctx.Err() != nil {
				result.cancelled = true
				break
			}

			if _, isConflict := conflicts[f.blobName]; isConflict {
				decision := policy
				if !explicitPolicy || (policy != conflictOverwriteAll && policy != conflictSkipAll) {
					reply := make(chan conflictAnswer, 1)
					select {
					case msgs <- uploadConflictMsg{blobName: f.blobName, reply: reply}:
					case <-ctx.Done():
						result.cancelled = true
					}
					if result.cancelled {
						break
					}
					select {
					case ans, ok := <-reply:
						if !ok {
							result.cancelled = true
						} else {
							decision = ans
							switch ans {
							case conflictOverwriteAll, conflictSkipAll:
								policy = ans
								explicitPolicy = true
							}
						}
					case <-ctx.Done():
						result.cancelled = true
					}
					if result.cancelled {
						break
					}
				}

				switch decision {
				case conflictCancel:
					result.cancelled = true
					if cancel != nil {
						cancel()
					}
				case conflictSkip, conflictSkipAll:
					result.skipped++
					continue
				case conflictOverwrite, conflictOverwriteAll:
					// fall through to upload
				}
				if result.cancelled {
					break
				}
			}

			var lastCum int64
			err := up.UploadBlob(ctx, f.blobName, f.localPath, func(cum int64) {
				delta := cum - lastCum
				lastCum = cum
				select {
				case msgs <- uploadProgressMsg{
					currentFile:  f.blobName,
					currentIndex: i,
					bytesDelta:   delta,
				}:
				default:
				}
			})
			if err != nil {
				result.failed = append(result.failed, uploadError{blobName: f.blobName, err: err})
				continue
			}
			result.uploaded++
			result.uploadedBytes += f.size
		}

		msgs <- result
	}()
}

// serviceUploader adapts *blob.Service to the uploader interface used
// by runUpload. Capturing account/container here keeps the per-file
// call sites minimal.
type serviceUploader struct {
	svc           *blob.Service
	account       blob.Account
	containerName string
}

func (s serviceUploader) ExistingBlobs(ctx context.Context, blobNames []string) (map[string]struct{}, error) {
	return s.svc.ExistingBlobs(ctx, s.account, s.containerName, blobNames)
}

func (s serviceUploader) UploadBlob(ctx context.Context, blobName, localPath string, progress func(int64)) error {
	return s.svc.UploadBlob(ctx, s.account, s.containerName, blobName, localPath, progress)
}

// uploadProgress captures the live state of an in-flight upload. Lives
// on the Model while the upload runs and drives the progress panel.
type uploadProgress struct {
	totalBytes        int64
	uploadedBytes     int64
	currentFile       string
	currentIndex      int
	total             int
	cancelled         bool
	done              bool
	waitingInput      bool      // true while a conflict prompt is shown
	waitingInputSince time.Time // stamped when waitingInput flips to true; zero otherwise
	errors            []uploadError
	skipped           int
	destPrefix        string
	bar               progress.Model

	// Throughput tracking. lastSampleAt / lastSampleBytes anchor the
	// instantaneous rate computation; bytesPerSec is an EMA so the
	// displayed speed doesn't flicker per-chunk.
	startedAt       time.Time
	lastSampleAt    time.Time
	lastSampleBytes int64
	bytesPerSec     float64
	finishedAt      time.Time // zero while in-flight; set by finishUpload
}

// pendingConflict captures the conflict prompt state: which blob name
// triggered the prompt and the reply channel the worker is blocked on.
type pendingConflict struct {
	blobName string
	reply    chan<- conflictAnswer
}

// updateUploadThroughput samples the byte counter at least 250ms after the
// previous sample and blends the instantaneous rate into an EMA so the
// displayed speed stays stable. Call after uploadedBytes has been advanced.
func (m *Model) updateUploadThroughput() {
	up := m.uploadProgress
	if up == nil {
		return
	}
	now := time.Now()
	delta := now.Sub(up.lastSampleAt)
	if delta < 250*time.Millisecond {
		return
	}
	bytesDelta := up.uploadedBytes - up.lastSampleBytes
	if bytesDelta < 0 {
		bytesDelta = 0
	}
	inst := float64(bytesDelta) / delta.Seconds()
	const alpha = 0.3
	if up.bytesPerSec == 0 {
		up.bytesPerSec = inst
	} else {
		up.bytesPerSec = alpha*inst + (1-alpha)*up.bytesPerSec
	}
	up.lastSampleAt = now
	up.lastSampleBytes = up.uploadedBytes
}

// openUploadBrowser mounts the file browser at the current working
// directory. Called from the action menu "Upload files..." entry.
func (m Model) openUploadBrowser() (Model, tea.Cmd) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "/"
	}
	m.uploadBrowser.Open(cwd, ui.OSDirReader{})
	m.uploadBrowserActive = true
	return m, nil
}

// startUpload kicks off the upload worker for selected paths with the
// given destination prefix. Builds the plan, creates the cancellable
// context, and returns the receive-next cmd.
func (m Model) startUpload(selected []string, destPrefix string) (Model, tea.Cmd) {
	if len(selected) == 0 {
		m.Notify(appshell.LevelInfo, "No files selected.")
		return m, nil
	}

	plan, err := planUpload(osUploadWalker{}, selected, destPrefix)
	if err != nil {
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to enumerate upload: %v", err))
		return m, nil
	}
	if len(plan.files) == 0 {
		m.Notify(appshell.LevelInfo, "No files to upload.")
		return m, nil
	}

	if m.uploadActivityUnreg != nil {
		m.uploadActivityUnreg()
		m.uploadActivityUnreg = nil
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	m.uploadCancelFn = cancelFn
	m.uploadConflictPolicy = conflictSkip

	bar := progress.New(progress.WithWidth(40))
	now := time.Now()
	m.uploadProgress = &uploadProgress{
		totalBytes:   plan.totalBytes,
		total:        len(plan.files),
		destPrefix:   destPrefix,
		bar:          bar,
		startedAt:    now,
		lastSampleAt: now,
	}

	activityID := ""
	if m.Activities != nil {
		act := &uploadActivity{
			progress: m.uploadProgress,
			id:       fmt.Sprintf("upload:%s:%d", destPrefix, now.UnixNano()),
			title:    fmt.Sprintf("%d files → %s", len(plan.files), formatUploadDest(destPrefix)),
			cancelFn: cancelFn,
		}
		m.uploadActivityUnreg = m.Activities.Register(act)
		activityID = act.id
	}

	up := serviceUploader{svc: m.service, account: m.currentAccount, containerName: m.containerName}
	msgs := make(chan tea.Msg, 16)
	runUpload(ctx, up, plan, destPrefix, cancelFn, msgs)
	return m, tea.Batch(
		newUploadCmd(msgs),
		requestActivityAutoOpen(activityID),
	)
}

// osUploadWalker is the production uploadWalker backed by os.Stat and
// filepath.WalkDir. Symlinks are not followed.
type osUploadWalker struct{}

func (osUploadWalker) Stat(path string) (bool, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}
	return info.IsDir(), info.Size(), nil
}

func (osUploadWalker) Walk(root string, fn func(string, bool, int64, error) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(path, false, 0, err)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return fn(path, true, 0, nil)
		}
		info, err := d.Info()
		if err != nil {
			return fn(path, false, 0, err)
		}
		return fn(path, false, info.Size(), nil)
	})
}

// finishUpload folds the terminal uploadDoneMsg into the progress panel
// and emits a summary notification. The panel auto-dismisses after 5s
// via the returned tea.Tick.
func (m Model) finishUpload(msg uploadDoneMsg) (Model, tea.Cmd) {
	if m.uploadProgress != nil {
		m.uploadProgress.done = true
		m.uploadProgress.cancelled = msg.cancelled
		m.uploadProgress.errors = msg.failed
		m.uploadProgress.skipped = msg.skipped
		m.uploadProgress.finishedAt = time.Now()
	}

	totalForMsg := 0
	if m.uploadProgress != nil {
		totalForMsg = m.uploadProgress.total
	}

	var (
		level appshell.NotificationLevel
		text  string
	)
	switch {
	case msg.cancelled:
		level = appshell.LevelInfo
		text = fmt.Sprintf("Cancelled — %d of %d uploaded (%s).", msg.uploaded, totalForMsg, humanSize(msg.uploadedBytes))
	case len(msg.failed) > 0:
		level = appshell.LevelWarn
		prefix := fmt.Sprintf("Uploaded %d of %d (%s). %d failed", msg.uploaded, totalForMsg, humanSize(msg.uploadedBytes), len(msg.failed))
		head := make([]string, 0, 3)
		for i, e := range msg.failed {
			if i >= 3 {
				break
			}
			head = append(head, e.blobName)
		}
		prefix += ": " + strings.Join(head, ", ")
		if len(msg.failed) > 3 {
			prefix += fmt.Sprintf(" (+ %d more)", len(msg.failed)-3)
		}
		text = prefix + "."
	default:
		level = appshell.LevelSuccess
		dest := msg.destPrefix
		if dest == "" {
			dest = "(root)"
		}
		text = fmt.Sprintf("Uploaded %d of %d (%s) to %s.", msg.uploaded, totalForMsg, humanSize(msg.uploadedBytes), dest)
	}
	m.Notify(level, text)

	return m, nil
}

// HasPendingUploadConflict reports whether a conflict prompt is
// currently waiting for the user to answer. Used by the parent app
// to decide whether to overlay the prompt on top of every other layer
// (including the activity overlay).
func (m Model) HasPendingUploadConflict() bool {
	return m.uploadConflict != nil
}

// RenderUploadConflictPrompt overlays the conflict prompt on top of
// base using the given screen dimensions. No-op when no conflict is
// pending. Exported so the parent app can render it after the ops
// center, keeping the modal at the top of the Z-stack.
func (m Model) RenderUploadConflictPrompt(base string, width, height int) string {
	if m.uploadConflict == nil {
		return base
	}
	cfg := ui.OverlayListConfig{
		Title:      fmt.Sprintf("%s already exists", m.uploadConflict.blobName),
		CloseHint:  "(y) overwrite · (n) skip · (a) overwrite all · (s) skip all · (c) cancel",
		MaxVisible: 0,
		HideSearch: true,
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, nil, 0, m.Styles, width, height, base)
}

// newUploadCmd returns a tea.Cmd that blocks on the next message in the
// msgs channel and returns it, carrying a `next` that chains the loop.
// Matches the pattern in cache.Broker.recv.
func newUploadCmd(msgs <-chan tea.Msg) tea.Cmd {
	var loop func() tea.Msg
	loop = func() tea.Msg {
		m, ok := <-msgs
		if !ok {
			return nil
		}
		switch v := m.(type) {
		case uploadStartedMsg:
			v.next = loop
			return v
		case uploadProgressMsg:
			v.next = loop
			return v
		case uploadConflictMsg:
			v.next = loop
			return v
		case uploadDoneMsg:
			return v
		}
		return m
	}
	return loop
}

// uploadActivity adapts a live *uploadProgress as an activity.Activity
// so the activity overlay can render it uniformly with broker fetches.
type uploadActivity struct {
	progress *uploadProgress
	id       string
	title    string
	cancelFn context.CancelFunc // snapshotted at creation; safe if nil
}

func (a *uploadActivity) ID() string          { return a.id }
func (a *uploadActivity) Kind() activity.Kind { return activity.KindUpload }
func (a *uploadActivity) Title() string       { return a.title }

func (a *uploadActivity) Snapshot() activity.Snapshot {
	up := a.progress
	if up == nil {
		return activity.Snapshot{Status: activity.StatusCancelled}
	}
	status := activity.StatusRunning
	switch {
	case up.done && up.cancelled:
		status = activity.StatusCancelled
	case up.done && len(up.errors) > 0:
		status = activity.StatusErrored
	case up.done:
		status = activity.StatusDone
	case up.waitingInput:
		status = activity.StatusWaitingInput
	}
	var firstErr error
	if len(up.errors) > 0 {
		firstErr = up.errors[0].err
	}
	return activity.Snapshot{
		Status:      status,
		StartedAt:   up.startedAt,
		FinishedAt:  up.finishedAt,
		TotalBytes:  up.totalBytes,
		DoneBytes:   up.uploadedBytes,
		Skipped:     up.skipped,
		BytesPerSec: up.bytesPerSec,
		Detail:      up.currentFile,
		Err:         firstErr,
	}
}

func (a *uploadActivity) Cancel() {
	if a.cancelFn != nil {
		a.cancelFn()
	}
}

// resolveConflict sends the user's answer to the upload worker and
// clears the conflict prompt state (including the activity's
// waitingInput flag).
func (m *Model) resolveConflict(answer conflictAnswer) {
	if m.uploadConflict != nil {
		m.uploadConflict.reply <- answer
		m.uploadConflict = nil
	}
	if m.uploadProgress != nil {
		// Roll the upload's clock forward by the time spent waiting on
		// the prompt — otherwise elapsed / ETA keep ticking during a
		// paused upload, which is misleading.
		if m.uploadProgress.waitingInput && !m.uploadProgress.waitingInputSince.IsZero() {
			pause := time.Since(m.uploadProgress.waitingInputSince)
			m.uploadProgress.startedAt = m.uploadProgress.startedAt.Add(pause)
			m.uploadProgress.lastSampleAt = m.uploadProgress.lastSampleAt.Add(pause)
		}
		m.uploadProgress.waitingInput = false
		m.uploadProgress.waitingInputSince = time.Time{}
	}
}

func formatUploadDest(destPrefix string) string {
	if destPrefix == "" {
		return "(root)"
	}
	return destPrefix
}
