// Package dockerx is the only place in the codebase that talks to a container
// runtime. It points at tecnativa/docker-socket-proxy, never at the socket
// itself, so the blast radius of a bug in here stops at create/start/exec/remove.
package dockerx

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	memoryBytes = 512 * 1024 * 1024
	nanoCPUs    = 500_000_000 // 0.5 CPU
	pidsLimit   = 256
	// Both mounts are wiped when the container goes. Nothing a student writes is
	// meant to outlive the session, and the check scripts run before it ends.
	tmpMount = "rw,noexec,nosuid,size=64m"
	// `exec` is spelled out because docker adds noexec to every --tmpfs unless
	// told otherwise, and a home that cannot run a file is a Linux course that
	// cannot teach `chmod +x`. Safe on its own terms: the mount is nosuid, the
	// root filesystem is read-only, every capability is dropped and there is no
	// network to fetch a binary from — the only thing runnable here is something
	// the student typed.
	homeMount = "rw,exec,nosuid,size=64m,uid=1000,gid=1000"
	homeDir   = "/home/student"
	// The image's own user. Repeated here so a rebuilt image that forgets USER
	// still cannot hand anyone root.
	runAsUser = "1000:1000"
)

type Runtime struct{ c *client.Client }

func New(host string) (*Runtime, error) {
	c, err := client.NewClientWithOpts(
		client.WithHost(host),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Runtime{c: c}, nil
}

func (r *Runtime) Close() error { return r.c.Close() }

// Create builds the container and starts it. Every limit is set here rather than
// in a config file: they are the difference between a sandbox and a shell on the
// host, so they are not something an operator should be able to loosen by
// mistake.
func (r *Runtime) Create(ctx context.Context, sessionID, image string) (string, error) {
	cfg := &container.Config{
		Image: image,
		User:  runAsUser,
		// Without this the prompt shows the container id, which tells a student
		// nothing and changes every session.
		Hostname: "devforge",
		// PID 1 has to outlive its own start: every terminal and every check
		// script arrives later as a separate exec.
		Cmd:        []string{"sleep", "infinity"},
		WorkingDir: homeDir,
		Tty:        false,
	}
	host := &container.HostConfig{
		Resources: container.Resources{
			Memory:    memoryBytes,
			NanoCPUs:  nanoCPUs,
			PidsLimit: ptr(int64(pidsLimit)),
		},
		CapDrop:        []string{"ALL"},
		SecurityOpt:    []string{"no-new-privileges"},
		ReadonlyRootfs: true,
		// A read-only root would also stop the student writing to their own home,
		// which is where most of the tasks ask them to work.
		Tmpfs:       map[string]string{"/tmp": tmpMount, homeDir: homeMount},
		NetworkMode: "none",
		AutoRemove:  false, // the reaper owns removal, so a crash still leaves a row to clean
	}

	res, err := r.c.ContainerCreate(ctx, cfg, host, nil, nil, ContainerName(sessionID))
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	if err := r.c.ContainerStart(ctx, res.ID, container.StartOptions{}); err != nil {
		// A container that will not start is still a container. Take it back out
		// rather than leaving the reaper to find it in an hour.
		_ = r.Remove(context.WithoutCancel(ctx), res.ID)
		return "", fmt.Errorf("start container: %w", err)
	}
	return res.ID, nil
}

func ContainerName(sessionID string) string { return "devforge-lab-" + sessionID }

func (r *Runtime) Remove(ctx context.Context, containerID string) error {
	err := r.c.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	if err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}

// Exec runs one command and waits for it. Used by the check scripts, where the
// exit code is the whole answer and the output is only there to show the student
// what happened.
func (r *Runtime) Exec(ctx context.Context, containerID string, cmd []string) (stdout string, exitCode int, err error) {
	id, err := r.c.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		User:         runAsUser,
		WorkingDir:   homeDir,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", 0, fmt.Errorf("exec create: %w", err)
	}

	att, err := r.c.ContainerExecAttach(ctx, id.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", 0, fmt.Errorf("exec attach: %w", err)
	}
	defer att.Close()

	var buf limitedBuffer
	// Without a TTY the stream is multiplexed, so it needs demultiplexing before
	// it reads as anything but noise.
	if _, err := stdcopy.StdCopy(&buf, &buf, att.Reader); err != nil {
		return "", 0, fmt.Errorf("exec read: %w", err)
	}

	insp, err := r.c.ContainerExecInspect(ctx, id.ID)
	if err != nil {
		return "", 0, fmt.Errorf("exec inspect: %w", err)
	}
	return buf.String(), insp.ExitCode, nil
}

// Attach opens an interactive shell with a TTY. The caller pipes the returned
// reader and writer at a websocket; closing the connection closes the exec, and
// the process inside dies with it.
func (r *Runtime) Attach(ctx context.Context, containerID string) (io.ReadWriteCloser, string, error) {
	id, err := r.c.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		// The rc file sits in /etc because the student's home is a tmpfs: anything
		// the image puts in there is mounted over before the shell starts.
		Cmd:          []string{"/bin/bash", "--rcfile", "/etc/devforge.bashrc"},
		User:         runAsUser,
		WorkingDir:   homeDir,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Env:          []string{"TERM=xterm-256color", "HOME=" + homeDir},
	})
	if err != nil {
		return nil, "", fmt.Errorf("terminal exec create: %w", err)
	}
	att, err := r.c.ContainerExecAttach(ctx, id.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		return nil, "", fmt.Errorf("terminal exec attach: %w", err)
	}
	return &hijacked{att.Conn, att.Reader}, id.ID, nil
}

// Resize keeps the shell's idea of the window in step with the browser's. Without
// it anything full-screen — nano, less, top — draws into the wrong box.
func (r *Runtime) Resize(ctx context.Context, execID string, rows, cols uint) error {
	return r.c.ContainerExecResize(ctx, execID, container.ResizeOptions{Height: rows, Width: cols})
}

// Alive reports whether the container is still there and running. A student who
// kills their own PID 1 gets a dead session rather than a terminal that silently
// accepts input and answers nothing.
func (r *Runtime) Alive(ctx context.Context, containerID string) bool {
	insp, err := r.c.ContainerInspect(ctx, containerID)
	return err == nil && insp.State != nil && insp.State.Running
}

// Ping fails fast at boot if the socket proxy is unreachable, rather than
// letting the first student to press Start find out.
func (r *Runtime) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := r.c.Ping(ctx); err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}
	return nil
}

func ptr[T any](v T) *T { return &v }
