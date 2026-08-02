package ui

import "time"

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
