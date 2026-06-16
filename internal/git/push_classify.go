package git

import "strings"

// PushFailureKind classifies a failed `git push` by its root cause, derived from
// the push's stderr. The set is deliberately small — false positives misdirect
// users worse than a missed signal does (spec §3.3).
type PushFailureKind string

const (
	KindProtectedBranch PushFailureKind = "protected_branch"
	KindAuth            PushFailureKind = "auth"
	KindNetwork         PushFailureKind = "network"
	KindUnknown         PushFailureKind = "unknown"
)

// ClassifyPushStderr maps stderr from a failed `git push` to a PushFailureKind.
// Matching is case-insensitive substring; first matching row wins.
func ClassifyPushStderr(stderr string) PushFailureKind {
	s := strings.ToLower(stderr)
	contains := func(needle string) bool { return strings.Contains(s, strings.ToLower(needle)) }

	if contains("remote rejected") {
		for _, marker := range []string{
			"gh006",
			"protected branch hook declined",
			"pre-receive hook declined",
			"protected",
		} {
			if contains(marker) {
				return KindProtectedBranch
			}
		}
	}

	for _, needle := range []string{
		"authentication failed",
		"permission denied (publickey)",
		"the requested url returned error: 403",
		"could not read username",
	} {
		if contains(needle) {
			return KindAuth
		}
	}

	for _, needle := range []string{
		"could not resolve host",
		"connection refused",
		"network is unreachable",
		"operation timed out",
	} {
		if contains(needle) {
			return KindNetwork
		}
	}

	return KindUnknown
}
