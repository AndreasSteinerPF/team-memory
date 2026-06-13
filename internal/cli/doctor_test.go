package cli

import "testing"

func TestAnyFailed(t *testing.T) {
	none := []checkResult{{sev: sevOK}, {sev: sevWarn}, {sev: sevSkip}}
	if anyFailed(none) {
		t.Error("no FAIL present → anyFailed should be false")
	}
	some := []checkResult{{sev: sevOK}, {sev: sevFail}}
	if !anyFailed(some) {
		t.Error("FAIL present → anyFailed should be true")
	}
}
