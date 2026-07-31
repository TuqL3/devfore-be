package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// tryTimeout bounds each of the two execs a trial does. Same ceiling the real
// grader uses, so a script that passes here cannot time out in front of a
// student.
const tryTimeout = checkTimeout

// TryResult is what the author sees. Unlike a student's check this carries the
// output: the whole point is to show why a script did not do what was meant.
type TryResult struct {
	SetupExitCode int
	SetupOutput   string
	ExitCode      int
	Output        string
	// True when the setup step failed, in which case the script's own result
	// says nothing useful — it ran against a container that was never prepared.
	SetupFailed bool
}

// TryScript runs a check script against a throwaway container built from the
// lab's own image. It exists because a script that is wrong fails students who
// did the task correctly, and there is no other way to find that out before
// publishing it.
//
// The container is fresh, so a bare script should FAIL: that is how an author
// catches the scripts that pass no matter what. Setup is the command that does
// the task, letting them check the other direction too.
func (l *Labs) TryScript(ctx context.Context, labID int64, setup, script string) (TryResult, error) {
	script = strings.TrimSpace(script)
	if script == "" {
		return TryResult{}, domain.ErrEmptyScript
	}

	image, err := l.grades.LabImage(ctx, labID)
	if err != nil {
		return TryResult{}, err
	}

	id, err := newSessionID()
	if err != nil {
		return TryResult{}, err
	}
	// Same limits as a student's container — no network, read-only root, dropped
	// capabilities. A trial is still someone's shell command running on a host.
	containerID, err := l.runtime.Create(ctx, "try-"+id, image)
	if err != nil {
		return TryResult{}, fmt.Errorf("tạo container thử: %w", err)
	}
	// Nothing in the database points at this container, so nothing else will
	// ever clean it up. Removal cannot depend on the request's context either:
	// a client that disconnects mid-trial would otherwise leak it.
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_ = l.runtime.Remove(rmCtx, containerID)
	}()

	var out TryResult
	if setup = strings.TrimSpace(setup); setup != "" {
		output, code, err := l.exec(ctx, containerID, setup)
		if err != nil {
			return TryResult{}, err
		}
		out.SetupOutput, out.SetupExitCode = output, code
		if code != 0 {
			// Reported rather than treated as an error: a failing setup command
			// is the author's typo, and they need to see which one it was.
			out.SetupFailed = true
			return out, nil
		}
	}

	output, code, err := l.exec(ctx, containerID, script)
	if err != nil {
		return TryResult{}, err
	}
	out.Output, out.ExitCode = output, code
	return out, nil
}

// exec runs one shell command the way the grader does, so a trial cannot pass
// on a shell the real check never uses.
func (l *Labs) exec(ctx context.Context, containerID, cmd string) (string, int, error) {
	runCtx, cancel := context.WithTimeout(ctx, tryTimeout)
	defer cancel()

	out, code, err := l.runtime.Exec(runCtx, containerID, []string{"/bin/sh", "-c", cmd})
	if err != nil {
		if runCtx.Err() != nil && ctx.Err() == nil {
			return "", 0, domain.ErrCheckTimeout
		}
		return "", 0, err
	}
	return out, code, nil
}
