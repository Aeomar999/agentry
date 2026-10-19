//go:build !race

package policy

// raceSlowdown is how much longer timed work may take.
const raceSlowdown = 1
