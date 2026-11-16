package provider

import "time"

// RotationRequested is rotationRequested, reachable from the external test package.
var RotationRequested = rotationRequested

// SetNameFreePoll makes a create waiting for a name try again every d, until
// the test ends.
func SetNameFreePoll(t interface{ Cleanup(func()) }, d time.Duration) {
	prev := nameFreePoll
	nameFreePoll = d
	t.Cleanup(func() { nameFreePoll = prev })
}
