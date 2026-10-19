//go:build !race

package web

// raceSlowdown is how much longer timed work may take: none without the
// race detector.
const raceSlowdown = 1
