//go:build race

package app

// raceDetector says this test binary was built with -race.
//
// The detector needs cgo, so the binary that job builds is deliberately not the one that
// ships, and the check on what ships has nothing to say about it.
const raceDetector = true
