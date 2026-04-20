package blobapp

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/activity"
	tea "charm.land/bubbletea/v2"
)

// fakeWalker is a synthetic fs for upload plan tests.
type fakeWalker struct {
	files map[string]int64    // absolute path -> size; presence = is file
	dirs  map[string][]string // absolute dir path -> child basenames (files + subdirs)
}

func (w *fakeWalker) Stat(path string) (bool, int64, error) {
	if size, ok := w.files[path]; ok {
		return false, size, nil
	}
	if _, ok := w.dirs[path]; ok {
		return true, 0, nil
	}
	return false, 0, nil
}

func (w *fakeWalker) Walk(root string, fn func(string, bool, int64, error) error) error {
	return w.walk(root, fn)
}

func (w *fakeWalker) walk(p string, fn func(string, bool, int64, error) error) error {
	if size, ok := w.files[p]; ok {
		return fn(p, false, size, nil)
	}
	children, ok := w.dirs[p]
	if !ok {
		return fn(p, false, 0, nil)
	}
	if err := fn(p, true, 0, nil); err != nil {
		return err
	}
	names := append([]string(nil), children...)
	sort.Strings(names)
	for _, name := range names {
		if err := w.walk(filepath.Join(p, name), fn); err != nil {
			return err
		}
	}
	return nil
}

func TestPlanUploadSingleFile(t *testing.T) {
	w := &fakeWalker{
		files: map[string]int64{"/home/user/report.csv": 1024},
	}
	plan, err := planUpload(w, []string{"/home/user/report.csv"}, "logs/2026")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.files) != 1 {
		t.Fatalf("want 1 file, got %d", len(plan.files))
	}
	if plan.files[0].blobName != "logs/2026/report.csv" {
		t.Fatalf("want logs/2026/report.csv, got %q", plan.files[0].blobName)
	}
	if plan.totalBytes != 1024 {
		t.Fatalf("want 1024 bytes, got %d", plan.totalBytes)
	}
}

func TestPlanUploadTrailingSlashEquivalence(t *testing.T) {
	w := &fakeWalker{files: map[string]int64{"/x/a.txt": 10}}
	a, _ := planUpload(w, []string{"/x/a.txt"}, "logs/2026")
	b, _ := planUpload(w, []string{"/x/a.txt"}, "logs/2026/")
	if a.files[0].blobName != b.files[0].blobName {
		t.Fatalf("trailing slash should match: %q vs %q", a.files[0].blobName, b.files[0].blobName)
	}
}

func TestPlanUploadRootDestination(t *testing.T) {
	w := &fakeWalker{files: map[string]int64{"/x/a.txt": 10}}
	plan, _ := planUpload(w, []string{"/x/a.txt"}, "")
	if plan.files[0].blobName != "a.txt" {
		t.Fatalf("want a.txt at root, got %q", plan.files[0].blobName)
	}
}

func TestPlanUploadFolderRecursive(t *testing.T) {
	w := &fakeWalker{
		files: map[string]int64{
			"/src/reports/q1/summary.csv": 100,
			"/src/reports/q1/raw.json":    200,
			"/src/reports/readme.md":      50,
		},
		dirs: map[string][]string{
			"/src/reports":    {"q1", "readme.md"},
			"/src/reports/q1": {"raw.json", "summary.csv"},
		},
	}
	plan, err := planUpload(w, []string{"/src/reports"}, "logs/2026")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	want := []string{
		"logs/2026/reports/q1/raw.json",
		"logs/2026/reports/q1/summary.csv",
		"logs/2026/reports/readme.md",
	}
	if len(plan.files) != len(want) {
		t.Fatalf("want %d files, got %d (%v)", len(want), len(plan.files), plan.blobNames())
	}
	for i, w := range want {
		if plan.files[i].blobName != w {
			t.Fatalf("file %d: want %q, got %q", i, w, plan.files[i].blobName)
		}
	}
	if plan.totalBytes != 350 {
		t.Fatalf("want 350 bytes, got %d", plan.totalBytes)
	}
}

func TestPlanUploadMixedSelection(t *testing.T) {
	w := &fakeWalker{
		files: map[string]int64{
			"/a.txt":       10,
			"/data/b.json": 20,
		},
		dirs: map[string][]string{"/data": {"b.json"}},
	}
	plan, err := planUpload(w, []string{"/a.txt", "/data"}, "bucket")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	want := []string{"bucket/a.txt", "bucket/data/b.json"}
	got := plan.blobNames()
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i, wn := range want {
		if got[i] != wn {
			t.Fatalf("blob %d: want %q got %q", i, wn, got[i])
		}
	}
}

func TestPlanUploadWindowsSeparatorsNormalized(t *testing.T) {
	w := &fakeWalker{files: map[string]int64{"/x/a.txt": 1}}
	plan, _ := planUpload(w, []string{"/x/a.txt"}, "logs\\2026")
	if plan.files[0].blobName != "logs/2026/a.txt" {
		t.Fatalf("want forward slashes, got %q", plan.files[0].blobName)
	}
}

// fakeUploader records calls and returns canned results per blobName.
type fakeUploader struct {
	uploads  []string // blob names called, in order
	existing map[string]struct{}
	failOn   map[string]error
	pauseOn  chan struct{} // if non-nil, every UploadBlob blocks on this
}

func (f *fakeUploader) ExistingBlobs(ctx context.Context, blobNames []string) (map[string]struct{}, error) {
	if f.existing == nil {
		return map[string]struct{}{}, nil
	}
	return f.existing, nil
}

func (f *fakeUploader) UploadBlob(ctx context.Context, blobName, localPath string, progress func(int64)) error {
	f.uploads = append(f.uploads, blobName)
	if f.pauseOn != nil {
		<-f.pauseOn
	}
	if err, ok := f.failOn[blobName]; ok {
		return err
	}
	if progress != nil {
		progress(10)
	}
	return nil
}

func TestUploadCommandEmitsStartAndDone(t *testing.T) {
	plan := uploadPlan{
		files: []fileToUpload{
			{localPath: "/x/a", blobName: "a", size: 10},
			{localPath: "/x/b", blobName: "b", size: 20},
		},
		totalBytes: 30,
	}
	up := &fakeUploader{}
	msgs := make(chan tea.Msg, 16)
	cancelFn := func() {}

	runUpload(context.Background(), up, plan, "logs/", cancelFn, msgs)

	timeout := time.After(1 * time.Second)
	var started, done bool
	for !done {
		select {
		case <-timeout:
			t.Fatalf("timeout waiting for messages; started=%v done=%v", started, done)
		case m := <-msgs:
			switch v := m.(type) {
			case uploadStartedMsg:
				started = true
				if v.fileCount != 2 || v.totalBytes != 30 {
					t.Fatalf("started: want fileCount=2 totalBytes=30, got %+v", v)
				}
			case uploadDoneMsg:
				done = true
				if v.uploaded != 2 {
					t.Fatalf("done: want uploaded=2, got %+v", v)
				}
			}
		}
	}
	if !started {
		t.Fatalf("never saw started msg")
	}
	if len(up.uploads) != 2 {
		t.Fatalf("want 2 uploads, got %d", len(up.uploads))
	}
}

func TestUploadConflictPolicyOverwriteAllSuppressesSecondPrompt(t *testing.T) {
	plan := uploadPlan{
		files: []fileToUpload{
			{localPath: "/x/a", blobName: "a", size: 10},
			{localPath: "/x/b", blobName: "b", size: 10},
		},
	}
	up := &fakeUploader{
		existing: map[string]struct{}{"a": {}, "b": {}},
	}
	msgs := make(chan tea.Msg, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runUpload(ctx, up, plan, "", cancel, msgs)

	var prompts int
	var done uploadDoneMsg
	timeout := time.After(1 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatalf("timeout; prompts=%d", prompts)
		case m, ok := <-msgs:
			if !ok {
				goto checks
			}
			switch v := m.(type) {
			case uploadConflictMsg:
				prompts++
				if prompts == 1 {
					v.reply <- conflictOverwriteAll
				} else {
					t.Fatalf("got a second prompt after OverwriteAll")
				}
			case uploadDoneMsg:
				done = v
			}
		}
	}
checks:
	if prompts != 1 {
		t.Fatalf("want 1 prompt, got %d", prompts)
	}
	if done.uploaded != 2 {
		t.Fatalf("want 2 uploads under OverwriteAll, got %d", done.uploaded)
	}
}

func TestUploadConflictPolicySkipAllCountsSkips(t *testing.T) {
	plan := uploadPlan{
		files: []fileToUpload{
			{localPath: "/x/a", blobName: "a", size: 10},
			{localPath: "/x/b", blobName: "b", size: 10},
		},
	}
	up := &fakeUploader{existing: map[string]struct{}{"a": {}, "b": {}}}
	msgs := make(chan tea.Msg, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runUpload(ctx, up, plan, "", cancel, msgs)

	var done uploadDoneMsg
	for m := range msgs {
		switch v := m.(type) {
		case uploadConflictMsg:
			v.reply <- conflictSkipAll
		case uploadDoneMsg:
			done = v
		}
	}
	if done.skipped != 2 {
		t.Fatalf("want skipped=2, got %+v", done)
	}
	if len(up.uploads) != 0 {
		t.Fatalf("want no uploads under SkipAll, got %v", up.uploads)
	}
}

func TestUploadConflictCancelAborts(t *testing.T) {
	plan := uploadPlan{
		files: []fileToUpload{
			{localPath: "/x/a", blobName: "a", size: 10},
			{localPath: "/x/b", blobName: "b", size: 10},
		},
	}
	up := &fakeUploader{existing: map[string]struct{}{"a": {}}}
	msgs := make(chan tea.Msg, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runUpload(ctx, up, plan, "", cancel, msgs)

	var done uploadDoneMsg
	for m := range msgs {
		switch v := m.(type) {
		case uploadConflictMsg:
			v.reply <- conflictCancel
		case uploadDoneMsg:
			done = v
		}
	}
	if !done.cancelled {
		t.Fatalf("want cancelled, got %+v", done)
	}
}

func TestUploadConflictSkipSinglePromptsAgain(t *testing.T) {
	plan := uploadPlan{
		files: []fileToUpload{
			{localPath: "/x/a", blobName: "a", size: 10},
			{localPath: "/x/b", blobName: "b", size: 10},
		},
	}
	up := &fakeUploader{existing: map[string]struct{}{"a": {}, "b": {}}}
	msgs := make(chan tea.Msg, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runUpload(ctx, up, plan, "", cancel, msgs)

	var prompts int
	for m := range msgs {
		if v, ok := m.(uploadConflictMsg); ok {
			prompts++
			v.reply <- conflictSkip
		}
	}
	if prompts != 2 {
		t.Fatalf("want 2 prompts (Skip single doesn't set policy), got %d", prompts)
	}
}

func TestUploadActivitySnapshotReflectsProgress(t *testing.T) {
	up := &uploadProgress{
		totalBytes:    1000,
		uploadedBytes: 250,
		total:         5,
		currentFile:   "a/b.txt",
		currentIndex:  1,
		startedAt:     time.Unix(100, 0),
		bytesPerSec:   42.0,
	}
	a := &uploadActivity{progress: up, id: "upload:test-1", title: "Upload 5 files"}

	if a.Kind() != activity.KindUpload {
		t.Fatalf("want KindUpload, got %v", a.Kind())
	}
	snap := a.Snapshot()
	if snap.TotalBytes != 1000 || snap.DoneBytes != 250 {
		t.Fatalf("bytes: want 1000/250, got %d/%d", snap.TotalBytes, snap.DoneBytes)
	}
	if snap.BytesPerSec != 42.0 {
		t.Fatalf("want rate 42, got %v", snap.BytesPerSec)
	}
	if snap.Status != activity.StatusRunning {
		t.Fatalf("want StatusRunning, got %v", snap.Status)
	}
}

func TestUploadActivitySnapshotMapsTerminalStates(t *testing.T) {
	// done=true without cancelled = Done.
	up := &uploadProgress{done: true, finishedAt: time.Unix(200, 0)}
	a := &uploadActivity{progress: up, id: "upload:t"}
	if got := a.Snapshot().Status; got != activity.StatusDone {
		t.Fatalf("done: want StatusDone, got %v", got)
	}

	// cancelled=true = Cancelled.
	up = &uploadProgress{done: true, cancelled: true, finishedAt: time.Unix(200, 0)}
	a = &uploadActivity{progress: up, id: "upload:t"}
	if got := a.Snapshot().Status; got != activity.StatusCancelled {
		t.Fatalf("cancelled: want StatusCancelled, got %v", got)
	}

	// done + any errors = Errored.
	up = &uploadProgress{done: true, errors: []uploadError{{blobName: "x", err: context.Canceled}}, finishedAt: time.Unix(200, 0)}
	a = &uploadActivity{progress: up, id: "upload:t"}
	if got := a.Snapshot().Status; got != activity.StatusErrored {
		t.Fatalf("errored: want StatusErrored, got %v", got)
	}

	// waitingInput=true (running upload blocked on prompt) = WaitingInput.
	up = &uploadProgress{waitingInput: true}
	a = &uploadActivity{progress: up, id: "upload:t"}
	if got := a.Snapshot().Status; got != activity.StatusWaitingInput {
		t.Fatalf("waitingInput: want StatusWaitingInput, got %v", got)
	}
}
