package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `truenas-power-manager — control backup TrueNAS power via IPMI

Commands (pick one):
  -power-on        Send power-on command via IPMI
  -power-off       Check backup status, then send graceful power-off (exits 1 if backup is running)
  -force-off       Send power-off unconditionally, without checking backup status
  -status          Print current IPMI chassis power state
  -backup-status   Print whether a replication task is currently running on the backup TrueNAS

Configuration is read from environment variables (see docker-compose.yml).
`

func main() {
	powerOn := flag.Bool("power-on", false, "")
	powerOff := flag.Bool("power-off", false, "")
	forceOff := flag.Bool("force-off", false, "")
	status := flag.Bool("status", false, "")
	backupStatus := flag.Bool("backup-status", false, "")

	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if !(*powerOn || *powerOff || *forceOff || *status || *backupStatus) {
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	ipmi := newIPMIController(cfg.IPMI)
	checker := newBackupChecker(cfg.TrueNAS)

	switch {
	case *status:
		state, err := ipmi.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Power status: %s\n", state)

	case *backupStatus:
		running, err := checker.IsBackupRunning()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if running {
			fmt.Println("Backup status: RUNNING")
		} else {
			fmt.Println("Backup status: IDLE")
		}

	case *powerOn:
		if err := ipmi.PowerOn(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Power-on command sent.")

	case *powerOff:
		running, err := checker.IsBackupRunning()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error checking backup status: %v\n", err)
			os.Exit(1)
		}
		if running {
			fmt.Fprintln(os.Stderr, "Backup is still running — power-off aborted.")
			os.Exit(1)
		}
		if err := ipmi.PowerOff(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Power-off command sent.")

	case *forceOff:
		if err := ipmi.PowerOff(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Power-off command sent (forced).")
	}
}
