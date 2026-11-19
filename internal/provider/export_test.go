package provider

import (
	"context"
	"time"
)

// RotationRequested is rotationRequested, reachable from the external test package.
var RotationRequested = rotationRequested

// SetNameFreePoll makes a create waiting for a name try again every d, until
// the test ends.
func SetNameFreePoll(t interface{ Cleanup(func()) }, d time.Duration) {
	prev := nameFreePoll
	nameFreePoll = d
	t.Cleanup(func() { nameFreePoll = prev })
}

// SetFirewallPoll makes a wait for the edge read again every d, until the
// test ends.
func SetFirewallPoll(t interface{ Cleanup(func()) }, d time.Duration) {
	prev := firewallPoll
	firewallPoll = d
	t.Cleanup(func() { firewallPoll = prev })
}

// CanonicalCIDR is canonicalCIDR, reachable from the external test package.
var CanonicalCIDR = canonicalCIDR

// CIDRSemanticallyEqual reports whether two spellings are one value to the
// type a firewall rule's sources are held in.
func CIDRSemanticallyEqual(a, b string) bool {
	equal, _ := newCIDRValue(a).StringSemanticEquals(context.Background(), newCIDRValue(b))
	return equal
}

// SetFirewallTimeout bounds a wait for the edge at d, until the test ends.
func SetFirewallTimeout(t interface{ Cleanup(func()) }, d time.Duration) {
	prev := firewallTimeout
	firewallTimeout = d
	t.Cleanup(func() { firewallTimeout = prev })
}
