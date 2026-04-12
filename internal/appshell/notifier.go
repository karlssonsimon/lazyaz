package appshell

import (
	"sync"
	"time"
)

// NotificationLevel categorizes a notification for color and sorting.
// The set is intentionally small — four levels covers the useful
// distinctions and avoids bikeshedding.
type NotificationLevel int

const (
	LevelInfo NotificationLevel = iota
	LevelSuccess
	LevelWarn
	LevelError
)

// String is for the history overlay's level pill.
func (l NotificationLevel) String() string {
	switch l {
	case LevelSuccess:
		return "OK"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Notification is one entry in the global log. Time is set at Push and
// drives both toast expiry and history sort order.
type Notification struct {
	Time      time.Time
	Level     NotificationLevel
	Message   string
	Spinner   bool // persistent loading notification — doesn't auto-expire
	SpinnerID int  // unique ID for resolving a specific spinner
}

// ToastDuration is how long each notification stays visible as a toast
// in the top-right corner before it's dropped from the active set.
const ToastDuration = 1500 * time.Millisecond

// ErrorToastDuration is the longer display time for error notifications.
const ErrorToastDuration = 5 * time.Second

// Notifier is the global notification store. It's a bounded ring — when
// the cap is exceeded, the oldest entry is evicted to make room. Safe
// for concurrent use, although in practice bubbletea's single-threaded
// Update loop means there's no real contention.
type Notifier struct {
	mu            sync.Mutex
	buf           []Notification
	cap           int
	nextSpinnerID int
}

// NewNotifier creates a Notifier with the given cap. Cap must be > 0.
func NewNotifier(capacity int) *Notifier {
	if capacity <= 0 {
		capacity = 1
	}
	return &Notifier{
		buf: make([]Notification, 0, capacity),
		cap: capacity,
	}
}

// Push appends a notification, evicting the oldest entry if the buffer
// is full. The notification's Time is set to time.Now().
func (n *Notifier) Push(level NotificationLevel, message string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pushLocked(Notification{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	})
}

// PushSpinner appends a persistent spinner notification that doesn't
// auto-expire. Returns a unique ID that can be passed to ResolveSpinner
// to replace it with a regular notification when the operation completes.
// Multiple spinners can be active concurrently.
func (n *Notifier) PushSpinner(message string) int {
	if n == nil {
		return 0
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.nextSpinnerID++
	id := n.nextSpinnerID
	n.pushLocked(Notification{
		Time:      time.Now(),
		Message:   message,
		Spinner:   true,
		SpinnerID: id,
	})
	return id
}

// DismissSpinner silently removes the spinner with the given ID without
// pushing a replacement notification. No-op if the spinner is not found.
func (n *Notifier) DismissSpinner(id int) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, entry := range n.buf {
		if entry.Spinner && entry.SpinnerID == id {
			n.buf = append(n.buf[:i], n.buf[i+1:]...)
			return
		}
	}
}

// ResolveSpinner finds the spinner with the given ID, removes it, and
// pushes a regular notification in its place. If the spinner is not
// found (already resolved or evicted), just pushes the notification.
func (n *Notifier) ResolveSpinner(id int, level NotificationLevel, message string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	// Remove the spinner entry.
	for i, entry := range n.buf {
		if entry.Spinner && entry.SpinnerID == id {
			n.buf = append(n.buf[:i], n.buf[i+1:]...)
			break
		}
	}
	n.pushLocked(Notification{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	})
}

func (n *Notifier) pushLocked(entry Notification) {
	if len(n.buf) >= n.cap {
		copy(n.buf, n.buf[1:])
		n.buf = n.buf[:len(n.buf)-1]
	}
	n.buf = append(n.buf, entry)
}

// Snapshot returns a copy of the full log, oldest first. Safe to
// iterate after the call returns — the returned slice is not aliased.
func (n *Notifier) Snapshot() []Notification {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Notification, len(n.buf))
	copy(out, n.buf)
	return out
}

// Active returns the notifications whose toast window has not yet
// expired, newest first (so the renderer can stack top-down without
// reversing). Spinner notifications are always included regardless of
// their age.
func (n *Notifier) Active(now time.Time) []Notification {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	var active []Notification
	for i := len(n.buf) - 1; i >= 0; i-- {
		entry := n.buf[i]
		if entry.Spinner {
			active = append(active, entry)
			continue
		}
		if entry.Time.Before(now.Add(-toastDurationFor(entry.Level))) {
			continue
		}
		active = append(active, entry)
	}
	return active
}

// Len returns the current number of stored notifications.
func (n *Notifier) Len() int {
	if n == nil {
		return 0
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.buf)
}

// HasActive reports whether any notification is still within its toast
// window, or any spinner is active.
func (n *Notifier) HasActive(now time.Time) bool {
	if n == nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := len(n.buf) - 1; i >= 0; i-- {
		entry := n.buf[i]
		if entry.Spinner {
			return true
		}
		if !entry.Time.Before(now.Add(-toastDurationFor(entry.Level))) {
			return true
		}
	}
	return false
}

func toastDurationFor(level NotificationLevel) time.Duration {
	if level == LevelError {
		return ErrorToastDuration
	}
	return ToastDuration
}
