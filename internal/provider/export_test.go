package provider

import "time"

// RotationRequested is rotationRequested, reachable from the external test package.
var RotationRequested = rotationRequested

// SetProjectNameFreePoll makes a create waiting for a name try again every d,
// until the test ends.
func SetProjectNameFreePoll(t interface{ Cleanup(func()) }, d time.Duration) {
	prev := projectNameFreePoll
	projectNameFreePoll = d
	t.Cleanup(func() { projectNameFreePoll = prev })
}
