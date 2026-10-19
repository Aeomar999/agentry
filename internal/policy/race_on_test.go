//go:build race

package policy

// raceSlowdown is how much longer timed work may take: the race detector
// makes judging a command some 20 times slower.
const raceSlowdown = 30
