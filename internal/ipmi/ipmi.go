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
	maxAttempts    = 3
	retryTimeout   = 30 * time.Minute
	retryInitDelay = 2 * time.Second
	retryMaxDelay  = 2 * time.Minute
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

// PowerOn sends the chassis power-on command.
// Safe to call when the server is already on.
// Retries with exponential backoff for up to retryTimeout on failure.
func (c *Controller) PowerOn() error {
	return c.withRetry("power on", func() error {
		state, err := c.Status()
		if err != nil {
			return err
		}
		if state == StateOn {
			return nil
		}
		if _, err := c.run("chassis", "power", "on"); err != nil {
			return fmt.Errorf("IPMI power on: %w", err)
		}
		return nil
	})
}

// PowerOff sends a graceful ACPI shutdown.
// Safe to call when the server is already off.
// Retries with exponential backoff for up to retryTimeout on failure.
func (c *Controller) PowerOff() error {
	return c.withRetry("power off", func() error {
		state, err := c.Status()
		if err != nil {
			return err
		}
		if state == StateOff {
			return nil
		}
		if _, err := c.run("chassis", "power", "soft"); err != nil {
			return fmt.Errorf("IPMI power off: %w", err)
		}
		return nil
	})
}

// withRetry calls fn repeatedly with exponential backoff until it succeeds or
// retryTimeout elapses. The delay starts at retryInitDelay and doubles each
// attempt, capped at retryMaxDelay.
func (c *Controller) withRetry(op string, fn func() error) error {
	deadline := time.Now().Add(retryTimeout)
	delay := retryInitDelay
	var lastErr error
	for attempt := 1; ; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if time.Now().Add(delay).After(deadline) {
			break
		}
		time.Sleep(delay)
		delay *= 2
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
	}
	return fmt.Errorf("IPMI %s failed after %s: %w", op, retryTimeout, lastErr)
}

// run executes ipmitool as a local subprocess. Arguments are passed as a slice
// so no shell quoting or injection is possible.
//
// Up to maxAttempts tries are made to handle the transient "auth type NONE not
// supported" negotiation failure that some BMCs emit on the first connection.
func (c *Controller) run(subcommand ...string) (string, error) {
	args := append(
		[]string{"-H", c.cfg.Host, "-U", c.cfg.User, "-P", c.cfg.Password, "-L", c.cfg.Privilege},
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
