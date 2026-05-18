package ipmi

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"truenas-power-manager/internal/config"
)

const (
	maxAttempts      = 3
	retryDelay       = 60 * time.Second
	pollInterval     = 10 * time.Second
	stateTimeout     = 10 * time.Minute
	maxStateAttempts = 3
)

// State represents the chassis power state.
type State int

const (
	StateUnknown State = iota
	StateOn
	StateOff
)

func (p State) String() string {
	switch p {
	case StateOn:
		return "ON"
	case StateOff:
		return "OFF"
	default:
		return "UNKNOWN"
	}
}

// Controller manages power by running ipmitool locally inside the container.
// The container must be on the same network as the IPMI LAN interface.
//
// ipmitool commands used:
//
//	chassis power status  — returns "Chassis Power is on/off"
//	chassis power on      — powers on
//	chassis power soft    — ACPI graceful shutdown
type Controller struct {
	cfg config.IPMIConfig
}

// New returns a new Controller for the given IPMI config.
func New(cfg config.IPMIConfig) *Controller {
	return &Controller{cfg: cfg}
}

// Status returns the current chassis power state.
func (c *Controller) Status() (State, error) {
	out, err := c.run("chassis", "power", "status")
	if err != nil {
		return StateUnknown, fmt.Errorf("IPMI power status: %w", err)
	}
	return parseStatus(out), nil
}

// PowerOn sends the chassis power-on command and waits until the system is ON.
// Retries the command up to maxStateAttempts times if the state isn't reached.
// Safe to call when the server is already on.
func (c *Controller) PowerOn() error {
	state, err := c.Status()
	if err != nil {
		return err
	}
	if state == StateOn {
		return nil
	}
	var lastErr error
	for attempt := 1; attempt <= maxStateAttempts; attempt++ {
		if _, err := c.run("chassis", "power", "on"); err != nil {
			return fmt.Errorf("IPMI power on: %w", err)
		}
		if lastErr = c.waitForState(StateOn); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("power on failed after %d attempts: %w", maxStateAttempts, lastErr)
}

// PowerOff sends a graceful ACPI shutdown and waits until the system is OFF.
// Retries the command up to maxStateAttempts times if the state isn't reached.
// Safe to call when the server is already off.
func (c *Controller) PowerOff() error {
	state, err := c.Status()
	if err != nil {
		return err
	}
	if state == StateOff {
		return nil
	}
	var lastErr error
	for attempt := 1; attempt <= maxStateAttempts; attempt++ {
		if _, err := c.run("chassis", "power", "soft"); err != nil {
			return fmt.Errorf("IPMI power off: %w", err)
		}
		if lastErr = c.waitForState(StateOff); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("power off failed after %d attempts: %w", maxStateAttempts, lastErr)
}

// waitForState polls Status every pollInterval until the chassis reaches target
// or stateTimeout elapses.
func (c *Controller) waitForState(target State) error {
	deadline := time.Now().Add(stateTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		state, err := c.Status()
		if err != nil {
			return fmt.Errorf("waiting for state %s: %w", target, err)
		}
		if state == target {
			return nil
		}
	}
	return fmt.Errorf("timed out after %s waiting for power state %s", stateTimeout, target)
}

// run executes ipmitool as a local subprocess. Arguments are passed as a slice
// so no shell quoting or injection is possible.
//
// Retries up to maxAttempts times with retryDelay between attempts to handle
// intermittent BMC LAN session failures that can last ~2 minutes.
func (c *Controller) run(subcommand ...string) (string, error) {
	args := append(
		[]string{"-I", c.cfg.Interface, "-A", c.cfg.AuthType, "-H", c.cfg.Host, "-U", c.cfg.User, "-P", c.cfg.Password, "-L", c.cfg.Privilege},
		subcommand...,
	)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command("ipmitool", args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		out := strings.TrimSpace(stdout.String())
		if err == nil {
			return out, nil
		}

		errMsg := strings.TrimSpace(stderr.String())
		errMsg = strings.ReplaceAll(errMsg, c.cfg.Password, "***")
		if errMsg != "" {
			lastErr = fmt.Errorf("ipmitool: %w — %s", err, errMsg)
		} else {
			lastErr = fmt.Errorf("ipmitool: %w", err)
		}

		if attempt < maxAttempts {
			time.Sleep(retryDelay)
		}
	}
	return "", lastErr
}

func parseStatus(output string) State {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "chassis power is on") {
		return StateOn
	}
	if strings.Contains(lower, "chassis power is off") {
		return StateOff
	}
	return StateUnknown
}
