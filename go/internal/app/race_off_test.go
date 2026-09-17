//go:build !race

package app

// raceDetector says this test binary was built with -race. See race_on_test.go.
const raceDetector = false
