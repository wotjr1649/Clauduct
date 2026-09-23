package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var reviewNoteMu sync.Mutex

// ReviewProbeNotes is overlay-only review instrumentation.
var ReviewProbeNotes []string

func reviewShort(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func (g *Gateway) reviewProbeNote(header, receipt, turn string, found bool) {
	var names []string
	entries, _ := os.ReadDir(filepath.Join(g.nativeEvents.directory, "active", "root"))
	for _, e := range entries {
		name := e.Name()
		if i := strings.IndexByte(name, '-'); i > 0 && len(name) > i+9 {
			name = name[:i+9] + filepath.Ext(name)
		}
		names = append(names, name)
	}
	reviewNoteMu.Lock()
	defer reviewNoteMu.Unlock()
	ReviewProbeNotes = append(ReviewProbeNotes, fmt.Sprintf("header=%s receipt=%s turn=%s found=%v active_root=%v", reviewShort(header), reviewShort(receipt), reviewShort(turn), found, names))
}
