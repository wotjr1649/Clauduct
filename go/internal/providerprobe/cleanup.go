package providerprobe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3CleanupNative is one native process the harness itself started to end a case's supervisor:
// the pid and creation time identify it, Run names the case and configuration that started it.
type p3CleanupNative struct {
	PID     uint32    `json:"pid"`
	Created time.Time `json:"created"`
	Run     string    `json:"run"`
	// Exited is the process's exact exit time, read from the retained process handle once it was
	// waited for. Zero while it may still run or when the handle could not say: a child is then
	// not proven to have had this generation as its parent. Never serialized.
	Exited time.Time `json:"-"`
}

// p3CleanupLog is every such process of a scope's runs, shared by its parallel cases. It is the
// only proof the verdict accepts that a connection to an earlier run's receiver was another
// case's cleanup and not this case's native.
type p3CleanupLog struct {
	mu      sync.Mutex
	natives []p3CleanupNative
}

// add records n and returns its index for exited.
func (l *p3CleanupLog) add(n p3CleanupNative) int {
	if l == nil {
		return -1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.natives = append(l.natives, n)
	return len(l.natives) - 1
}

// exited records the exact exit time of the process add returned i for.
func (l *p3CleanupLog) exited(i int, at time.Time) {
	if l == nil || i < 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < len(l.natives) {
		l.natives[i].Exited = at
	}
}

func (l *p3CleanupLog) all() []p3CleanupNative {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]p3CleanupNative(nil), l.natives...)
}

// p3StopOwnSupervisor stops the native supervisor of one probe config dir and its sessions, and
// waits until no native of that dir is left; a process still running is reported, not ended.
// The stop native is recorded in log (run names the case) before it is waited for.
func p3StopOwnSupervisor(exe, configDir string, log *p3CleanupLog, run string) error {
	cmd := exec.Command(exe, "daemon", "stop", "--any")
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir, "DISABLE_AUTOUPDATER=1")
	cmd.Dir = configDir
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err := p3RunRecorded(ctx, cmd, log, run)
	cancel()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		// An incomplete census cannot show the dir empty.
		if left, complete := probeNativeCensus(exe, configDir, time.Time{}, nil); complete && len(left) == 0 {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("natives of the probe config dir still running, or not all readable, after the supervisor stopped")
}

// p3RunRecorded is childprocess.Run that records the process's pid and creation time right
// after it starts. A process too short-lived to be read is recorded with a zero creation time,
// which proves nothing about any connection it made.
func p3RunRecorded(ctx context.Context, cmd *exec.Cmd, log *p3CleanupLog, run string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := childprocess.Start(cmd)
	if err != nil {
		return err
	}
	p.RecordReap()
	n := p3CleanupNative{PID: uint32(p.PID()), Run: run}
	if id, err := p.Identity(); err == nil {
		n.Created = time.Unix(0, int64(id.Creation-116444736000000000)*100)
	}
	index := log.add(n)
	stop := context.AfterFunc(ctx, func() { _ = p.Stop() })
	defer stop()
	err = p.Wait()
	// The stop native's own Job reports to the run through the error: a held member or a Job not
	// confirmed empty is a cleanup failure.
	if held, _, reapErr := p.JobEmpty(); held > 0 || reapErr != nil {
		err = errors.Join(err, fmt.Errorf("HELD_PROCESS or JOB_UNCONFIRMED: %d held, %v", held, reapErr))
	}
	if !n.Created.IsZero() && cmd.ProcessState != nil {
		if usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok && usage != nil {
			log.exited(index, time.Unix(0, usage.ExitTime.Nanoseconds()))
		}
	}
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	return err
}

// p3PriorContact is one earlier run's receiver this case's window reached: which receiver, the
// run that created it, what it counted since this case began (bytes and connections apart) and
// the owners the Counter recorded for those connections. Tunnel counts CONNECTs that went
// through CATCH_ALL to the receiver's address, whose owner nothing records.
type p3PriorContact struct {
	Receiver string                   `json:"receiver"`
	URL      string                   `json:"url"`
	Source   string                   `json:"sourceRun"`
	Bytes    int64                    `json:"bytes"`
	Conns    int64                    `json:"conns"`
	Tunnel   int64                    `json:"tunnel,omitempty"`
	Total    int                      `json:"total,omitempty"`
	Owners   []providercert.ConnOwner `json:"owners,omitempty"`
}

// p3PriorCrossTalk reports whether every prior-receiver contact of the run self is accounted for
// by another case's cleanup native. Cross-talk is proven only when, for each contact, there is
// no tunnel, there is at least one connection, every connection has a recorded owner (the
// Counter keeps a bounded number), and every owner is a cleanup native another run started:
// the same pid and creation time, or a child whose parent pid is one and that was created within
// that process's exact lifetime (creation through the exit time its handle reported); a child of
// a pid whose generation exit is unknown is not proven. Anything else, no
// owner, an owner that is this run's own cleanup or a native nobody recorded, is not cross-talk
// and the contact fails routing. A contact of this case's own native is never cross-talk.
func p3PriorCrossTalk(contacts []p3PriorContact, cleanups []p3CleanupNative, self string) bool {
	if len(contacts) == 0 {
		return false
	}
	for _, c := range contacts {
		if c.Tunnel != 0 || c.Conns <= 0 || int64(len(c.Owners)) != c.Conns {
			return false
		}
		for _, o := range c.Owners {
			if o.PID == 0 || o.Created.IsZero() {
				return false
			}
			proven := false
			for _, n := range cleanups {
				if n.PID == 0 || n.Created.IsZero() {
					continue
				}
				child := o.ParentPID == n.PID && !n.Exited.IsZero() && !o.Created.Before(n.Created) && !o.Created.After(n.Exited)
				if (o.PID == n.PID && o.Created.Equal(n.Created)) || child {
					if n.Run == self {
						return false
					}
					proven = true
				}
			}
			if !proven {
				return false
			}
		}
	}
	return true
}
