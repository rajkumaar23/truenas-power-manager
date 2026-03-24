package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ipmiController manages power by running ipmitool locally inside the container.
// The container must be on the same network as the IPMI LAN interface.
//
// ipmitool commands used:
//
//	chassis power status  — returns "Chassis Power is on/off"
//	chassis power on      — powers on
//	chassis power soft    — ACPI graceful shutdown
type ipmiController struct {
	cfg IPMIConfig
}

func newIPMIController(cfg IPMIConfig) *ipmiController {
	return &ipmiController{cfg: cfg}
}

// Status returns the current chassis power state.
func (c *ipmiController) Status() (PowerState, error) {
	out, err := c.runIPMITool("chassis", "power", "status")
	if err != nil {
		return PowerUnknown, fmt.Errorf("IPMI power status: %w", err)
	}
	return parsePowerStatus(out), nil
}

// PowerOn sends the chassis power-on command.
// Safe to call when the server is already on.
func (c *ipmiController) PowerOn() error {
	state, err := c.Status()
	if err != nil {
		return err
	}
	if state == PowerOn {
		return nil
	}
	if _, err := c.runIPMITool("chassis", "power", "on"); err != nil {
		return fmt.Errorf("IPMI power on: %w", err)
	}
	return nil
}

// PowerOff sends a graceful ACPI shutdown.
// Safe to call when the server is already off.
func (c *ipmiController) PowerOff() error {
	state, err := c.Status()
	if err != nil {
		return err
	}
	if state == PowerOff {
		return nil
	}
	if _, err := c.runIPMITool("chassis", "power", "soft"); err != nil {
		return fmt.Errorf("IPMI power off: %w", err)
	}
	return nil
}

// runIPMITool runs ipmitool as a local subprocess. Arguments are passed as a
// slice so no shell quoting or injection is possible.
func (c *ipmiController) runIPMITool(subcommand ...string) (string, error) {
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
		// Redact password from any error output before surfacing it.
		errMsg = strings.ReplaceAll(errMsg, c.cfg.Password, "***")
		if errMsg != "" {
			return "", fmt.Errorf("ipmitool: %w — %s", err, errMsg)
		}
		return "", fmt.Errorf("ipmitool: %w", err)
	}
	return out, nil
}

// PowerState represents the chassis power state.
type PowerState int

const (
	PowerUnknown PowerState = iota
	PowerOn
	PowerOff
)

func (p PowerState) String() string {
	switch p {
	case PowerOn:
		return "ON"
	case PowerOff:
		return "OFF"
	default:
		return "UNKNOWN"
	}
}

func parsePowerStatus(output string) PowerState {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "chassis power is on") {
		return PowerOn
	}
	if strings.Contains(lower, "chassis power is off") {
		return PowerOff
	}
	return PowerUnknown
}
