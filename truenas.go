package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// backupChecker queries a TrueNAS host to determine whether any replication
// tasks are still running. Point it at the source, since replication is a push
// and task state is only tracked there.
type backupChecker struct {
	cfg TrueNASConfig
}

func newBackupChecker(cfg TrueNASConfig) *backupChecker {
	return &backupChecker{cfg: cfg}
}

// IsBackupRunning returns true if any replication task is actively running.
// It uses midclt (TrueNAS middleware CLI) over SSH to query task state.
// Falls back to checking for active zfs send processes if midclt is unavailable.
func (b *backupChecker) IsBackupRunning() (bool, error) {
	client, err := newSSHClient(b.cfg.Host, b.cfg.Port, b.cfg.User, b.cfg.Password, b.cfg.KeyFile)
	if err != nil {
		return false, fmt.Errorf("connecting to TrueNAS at %s: %w", b.cfg.Host, err)
	}
	defer client.close()

	// Primary method: query replication tasks via TrueNAS middleware.
	running, err := b.checkReplicationTasks(client)
	if err == nil {
		return running, nil
	}

	// Fallback: check for active zfs send/recv processes.
	return b.checkZFSProcesses(client)
}

// replicationTask is a minimal representation of a TrueNAS replication task.
type replicationTask struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State struct {
		State string `json:"state"`
	} `json:"state"`
}

// checkReplicationTasks queries midclt for running replication tasks.
func (b *backupChecker) checkReplicationTasks(client *sshClient) (bool, error) {
	// midclt call returns JSON array of task objects.
	out, err := client.run(`midclt call replication.query '[]'`)
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

// checkZFSProcesses falls back to checking for live zfs send/recv processes.
func (b *backupChecker) checkZFSProcesses(client *sshClient) (bool, error) {
	// pgrep exits 0 if matches found, 1 if not — use runIgnoreExitCode.
	out, err := client.runIgnoreExitCode(`pgrep -f "zfs (send|recv)"`)
	if err != nil {
		return false, fmt.Errorf("pgrep zfs: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}
