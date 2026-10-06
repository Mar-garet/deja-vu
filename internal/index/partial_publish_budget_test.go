package index

import (
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// The early index is a stopgap, so it has to be cheap whatever the newest
// sessions weigh. Counting sessions alone let 200 long recent ones hold more
// text than the rest of the store, and the stopgap answered later than a build
// without it finished (#4768).
func TestNewestSliceStopsAtTheMessageBudget(t *testing.T) {
	sess := func(msgs int) model.Session { return model.Session{Messages: make([]model.Message, msgs)} }
	long := make([]model.Session, 300)
	for i := range long {
		long[i] = sess(5000)
	}
	if n := newestSlice(long); n*5000 > partialPublishMessages || n < 1 {
		t.Errorf("200 long sessions: took %d (%d messages), budget %d", n, n*5000, partialPublishMessages)
	}
	short := make([]model.Session, 300)
	for i := range short {
		short[i] = sess(10)
	}
	if n := newestSlice(short); n != partialPublishSessions {
		t.Errorf("short sessions: took %d, want the %d-session cap", n, partialPublishSessions)
	}
	// The newest session goes in even when it alone is over the budget.
	if n := newestSlice([]model.Session{sess(partialPublishMessages * 3), sess(10)}); n != 1 {
		t.Errorf("oversized newest session: took %d, want 1", n)
	}
}
