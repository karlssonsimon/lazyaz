package ui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// SpinnerMinVisible is the minimum duration the spinner stays visible once
// a load starts. Loads that complete faster than this are artificially held
// until the threshold elapses, so the user always sees a spinner flash
// confirming that a fetch ran.
const SpinnerMinVisible = 400 * time.Millisecond

// spinnerFrames are the animation frames (MiniDot variant — well supported
// across terminals). Each frame is a single-character braille spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerFPS is the target frame rate of the spinner.
const spinnerFPS = 80 * time.Millisecond

// SpinnerFrameAt returns the spinner frame character for the given elapsed
// duration. Computing frames from elapsed time avoids tick-chain state bugs
// — any render after ticks start will show the correct current frame.
func SpinnerFrameAt(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	idx := int(elapsed/spinnerFPS) % len(spinnerFrames)
	return spinnerFrames[idx]
}

// RenderPaneSpinner right-aligns a spinner onto a pane title string when
// loading is true. When not loading, the title is returned unchanged.
//
// The bubbles list wraps the title with its Title style (Padding(0,1) = 2),
// appends "  " for the status message, and truncates to listWidth-1. So the
// usable content width is listWidth-5. This function accounts for that.
func RenderPaneSpinner(title string, loading bool, startedAt time.Time, styles Styles, width int) string {
	if !loading {
		return title
	}
	spin := styles.Accent.Render(SpinnerFrameAt(time.Since(startedAt)))
	titleW := lipgloss.Width(title)
	spinW := lipgloss.Width(spin)
	target := width - 5
	gap := target - titleW - spinW
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + spin
}
