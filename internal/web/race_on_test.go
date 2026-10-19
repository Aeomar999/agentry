//go:build race

package web

// raceSlowdown is how much longer timed work may take: the race detector
// makes reading and folding a task's events many times slower.
const raceSlowdown = 30
