package ipmi

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"truenas-power-manager/internal/config"
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
func (c *Controller) PowerOn() error {
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
}

// PowerOff sends a graceful ACPI shutdown.
// Safe to call when the server is already off.
func (c *Controller) PowerOff() error {
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
}

// run executes ipmitool as a local subprocess. Arguments are passed as a slice
// so no shell quoting or injection is possible.
func (c *Controller) run(subcommand ...string) (string, error) {
	args := append(
		[]string{"-H", c.cfg.Host, "-U", c.cfg.User, "-P", c.cfg.Password, "-L", c.cfg.Privilege},
		subcommand...,
	)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("ipmitool", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		errMsg = strings.ReplaceAll(errMsg, c.cfg.Password, "***")
		if errMsg != "" {
			return "", fmt.Errorf("ipmitool: %w — %s", err, errMsg)
		}
		return "", fmt.Errorf("ipmitool: %w", err)
	}
	return out, nil
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
