package childprocess

import (
	"io"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

// slowReader is still delivering input when the child is already gone.
type slowReader struct {
	inRead atomic.Bool
	done   bool
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.inRead.Store(true)
	time.Sleep(300 * time.Millisecond)
	r.inRead.Store(false)
	r.done = true
	return 0, io.EOF
}

// exec.Cmd.Wait waits for its stdin goroutine and reports its error; this type imitates
// that contract, and the stdin copier was the one pipe goroutine left out of the group
// Wait() waits on. Wait() then closed p.pipes while the copy was still writing to one.
//
// A child that exits without reading is the ordinary way to reach it -- here by exiting
// immediately -- so the check is simply that Wait() does not return while the reader is
// still being read.
func TestWaitDoesNotReturnWhileStdinIsStillBeingRead(t *testing.T) {
	reader := &slowReader{}
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	cmd.Stdin = reader
	p, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("a child that ignored its input reported a failure: %v", err)
	}
	if reader.inRead.Load() {
		t.Fatal("Wait returned with the stdin copy still running, so the pipe it writes to was closed under it")
	}
}
