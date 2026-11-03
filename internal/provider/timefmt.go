package provider

import "time"

// rfc3339OrEmpty renders an optional time as the provider shows every
// timestamp, and an absent one as the empty string a computed attribute reads
// as "nothing pending".
func rfc3339OrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
