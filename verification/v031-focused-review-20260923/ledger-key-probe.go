package gateway

import (
	"fmt"
	"sync"
)

var reviewProbeMu sync.Mutex

// ReviewProbeKeys is overlay-only review instrumentation.
var ReviewProbeKeys []string

func reviewProbeRecord(key nativeExecutionKey) {
	reviewProbeMu.Lock()
	defer reviewProbeMu.Unlock()
	turn := key.turn
	if len(turn) > 8 {
		turn = turn[:8]
	}
	ReviewProbeKeys = append(ReviewProbeKeys, fmt.Sprintf("agent=%q turn=%s step=%d class=%s", key.agent, turn, key.step, key.class))
}
