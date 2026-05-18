package runtime_deploy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DockerDriver shells out to the local `docker` CLI. It does NOT use the
// docker SDK directly to avoid pulling a 30MB+ dependency for what is
// effectively a five-subprocess driver. This keeps the binary lean and the
// behavior auditable: every container action is a real subprocess whose
// output we can log or inspect.
type DockerDriver struct {
	// CommandTimeout caps how long any single docker subprocess can take.
	CommandTimeout time.Duration
}

// NewDockerDriver returns a driver with sensible defaults.
func NewDockerDriver() *DockerDriver {
	return &DockerDriver{CommandTimeout: 60 * time.Second}
}

// ErrDockerUnavailable means the local docker CLI isn't usable. The engine
// refuses to start in this state — silently no-oping would be worse than
// a loud failure.
var ErrDockerUnavailable = errors.New("docker CLI is not available or daemon unreachable")

// Available probes whether `docker version` returns successfully. Called once
// at engine startup; the result is surfaced through the runtime status API.
func (d *DockerDriver) Available(ctx context.Context) error {
	out, err := d.run(ctx, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return fmt.Errorf("%w: %v: %s", ErrDockerUnavailable, err, strings.TrimSpace(out))
	}
	return nil
}

// Pull pulls an image. Returns the raw docker output for the event log.
func (d *DockerDriver) Pull(ctx context.Context, image string) (string, error) {
	return d.run(ctx, "pull", image)
}

// Run starts a detached container. Returns the container ID.
// Uses --restart=no so the engine remains the authority on lifecycle.
func (d *DockerDriver) Run(ctx context.Context, containerName, image string, hostPort, containerPort int) (string, error) {
	args := []string{
		"run", "-d",
		"--name", containerName,
		"--restart", "no",
		"--label", "autostack.runtime=true",
		"--label", "autostack.deployment_name=" + containerName,
	}
	if hostPort > 0 && containerPort > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:%d", hostPort, containerPort))
	}
	args = append(args, image)
	out, err := d.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("docker run failed: %w: %s", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// InspectStatus returns the running status of a container, or empty string if
// the container is not found.
func (d *DockerDriver) InspectStatus(ctx context.Context, containerID string) (string, error) {
	out, err := d.run(ctx, "inspect", "--format", "{{.State.Status}}", containerID)
	if err != nil {
		if strings.Contains(out, "No such object") {
			return "", nil
		}
		return "", fmt.Errorf("docker inspect failed: %w: %s", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// InspectImage returns the image currently in use by a container.
// Used by the drift loop to detect a mismatch between desired and observed.
func (d *DockerDriver) InspectImage(ctx context.Context, containerID string) (string, error) {
	out, err := d.run(ctx, "inspect", "--format", "{{.Config.Image}}", containerID)
	if err != nil {
		return "", fmt.Errorf("docker inspect (image) failed: %w: %s", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// Logs returns the last N log lines from a container.
func (d *DockerDriver) Logs(ctx context.Context, containerID string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	out, err := d.run(ctx, "logs", "--tail", fmt.Sprintf("%d", tail), containerID)
	if err != nil {
		return out, fmt.Errorf("docker logs failed: %w", err)
	}
	return out, nil
}

// Stop stops a running container (SIGTERM with 5s timeout, then SIGKILL).
func (d *DockerDriver) Stop(ctx context.Context, containerID string) error {
	out, err := d.run(ctx, "stop", "-t", "5", containerID)
	if err != nil {
		return fmt.Errorf("docker stop failed: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Remove force-removes a container.
func (d *DockerDriver) Remove(ctx context.Context, containerID string) error {
	out, err := d.run(ctx, "rm", "-f", containerID)
	if err != nil {
		if strings.Contains(out, "No such container") {
			return nil
		}
		return fmt.Errorf("docker rm failed: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// run executes `docker <args...>` with a bounded timeout and returns
// combined stdout/stderr. Never returns nil output even on error.
func (d *DockerDriver) run(ctx context.Context, args ...string) (string, error) {
	timeout := d.CommandTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
