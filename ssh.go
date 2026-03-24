package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshClient wraps an SSH session for running one-off commands.
type sshClient struct {
	client  *ssh.Client
	secrets []string // redacted from all error messages
}

func (c *sshClient) redact(s string) string {
	for _, secret := range c.secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "***")
		}
	}
	return s
}

func newSSHClient(host string, port int, user, password, keyFile string) (*sshClient, error) {
	authMethods, err := buildAuthMethods(password, keyFile)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // trusted internal network
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s: %w", addr, err)
	}

	return &sshClient{client: client, secrets: []string{password}}, nil
}

func (c *sshClient) close() {
	c.client.Close()
}

// run executes a command and returns trimmed stdout.
// Any secrets registered on the client are redacted from error messages.
func (c *sshClient) run(cmd string) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new SSH session: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err := sess.Run(cmd); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("command %q: %w — stderr: %s", c.redact(cmd), err, c.redact(errMsg))
		}
		return "", fmt.Errorf("command %q: %w", c.redact(cmd), err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// runIgnoreExitCode runs a command and returns stdout even if the exit code is non-zero.
// Useful for commands like pgrep that exit 1 when nothing is found.
func (c *sshClient) runIgnoreExitCode(cmd string) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new SSH session: %w", err)
	}
	defer sess.Close()

	var stdout bytes.Buffer
	sess.Stdout = &stdout
	sess.Run(cmd) // intentionally ignore error

	return strings.TrimSpace(stdout.String()), nil
}


func buildAuthMethods(password, keyFile string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if keyFile != "" {
		key, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("reading SSH key %q: %w", keyFile, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parsing SSH key %q: %w", keyFile, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if password != "" {
		methods = append(methods, ssh.Password(password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no authentication method provided")
	}

	return methods, nil
}
