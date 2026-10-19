//go:build race

package storetest

// equalEvery is how many random steps go between replay checks: the race
// detector makes each replay some 30 times slower.
const equalEvery = 10
