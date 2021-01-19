package stressgen

import (
	"math/rand"
)

// newRand is a convenience wrapper around our common operation of constructing
// a random source with a particular seed and then wrapping it in a *rand.Rand
// object for more convenient use.
func newRand(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}
