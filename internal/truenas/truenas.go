package truenas

import (
	"encoding/json"
	"fmt"
	"strings"

	"truenas-power-manager/internal/config"
	"truenas-power-manager/internal/transport"
)

// BackupChecker queries a TrueNAS host to determine whether any replication
// tasks are still running. Point it at the source, since replication is a push
// and task state is only tracked there.
type BackupChecker struct {
	cfg config.TrueNASConfig
}

// New returns a new BackupChecker for the given TrueNAS config.
func New(cfg config.TrueNASConfig) *BackupChecker {
	return &BackupChecker{cfg: cfg}
}

// IsBackupRunning returns true if any replication task is actively running.
// It uses midclt (TrueNAS middleware CLI) over SSH to query task state.
// Falls back to checking for active zfs send processes if midclt is unavailable.
func (b *BackupChecker) IsBackupRunning() (bool, error) {
	client, err := transport.NewClient(b.cfg.Host, b.cfg.Port, b.cfg.User, b.cfg.Password, b.cfg.KeyFile)
	if err != nil {
		return false, fmt.Errorf("connecting to TrueNAS at %s: %w", b.cfg.Host, err)
	}
	defer client.Close()

	running, err := b.checkReplicationTasks(client)
	if err == nil {
		return running, nil
	}

	// Fallback: check for active zfs send/recv processes.
	return b.checkZFSProcesses(client)
}

type replicationTask struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State struct {
		State string `json:"state"`
	} `json:"state"`
}

func (b *BackupChecker) checkReplicationTasks(client *transport.Client) (bool, error) {
	out, err := client.Run(`midclt call replication.query '[]'`)
	if err != nil {
		return false, fmt.Errorf("midclt replication.query: %w", err)
	}

	var tasks []replicationTask
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		return false, fmt.Errorf("parsing replication task JSON: %w", err)
	}

	for _, t := range tasks {
		if strings.EqualFold(t.State.State, "RUNNING") {
			return true, nil
		}
	}
	return false, nil
}

func (b *BackupChecker) checkZFSProcesses(client *transport.Client) (bool, error) {
	// pgrep exits 0 if matches found, 1 if not — use RunIgnoreExitCode.
	out, err := client.RunIgnoreExitCode(`pgrep -f "zfs (send|recv)"`)
	if err != nil {
		return false, fmt.Errorf("pgrep zfs: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}
