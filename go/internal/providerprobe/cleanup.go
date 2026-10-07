package providerprobe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
)

// p3StopOwnSupervisor stops the native supervisor of one probe config dir and its sessions, and
// waits until no native of that dir is left; a process still running is reported, not ended.
func p3StopOwnSupervisor(exe, configDir string) error {
	cmd := exec.Command(exe, "daemon", "stop", "--any")
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir, "DISABLE_AUTOUPDATER=1")
	cmd.Dir = configDir
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err := childprocess.Run(ctx, cmd)
	cancel()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if len(probeNatives(exe, configDir, time.Time{}, nil)) == 0 {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("natives of the probe config dir still running after the supervisor stopped")
}
