package recipes

import (
	"strings"
	"testing"
)

// A stated time too large to be a step is refused, never wrapped round into
// a short one by an overflowing conversion.
func TestStatedDurationRefusesHugeTimes(t *testing.T) {
	huge := strings.Repeat("9", 400)
	for _, text := range []string{
		"beat for " + huge + " seconds",
		"simmer " + huge + " minutes",
		"rest 99999999999999999999 seconds",
	} {
		if d, ok := statedDuration(text); ok {
			t.Errorf("statedDuration(%.40q…) = %+v, want refused", text, d)
		}
	}
}
