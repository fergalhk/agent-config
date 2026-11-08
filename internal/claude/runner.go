package claude

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Runner runs the Claude Code CLI once, without interaction.
type Runner struct {
	// Bin is the name or path of the Claude Code binary.
	Bin string
	// Timeout limits the run time. Zero means no limit.
	Timeout time.Duration
	// Env holds extra environment variables, in KEY=value form.
	Env []string
}

// Run runs Claude Code in repoDir with the given prompt.
// Claude Code may read scratchDir. Its output goes to the output of this process.
func (r Runner) Run(ctx context.Context, repoDir, scratchDir, prompt string) error {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, r.Bin,
		"-p", prompt,
		"--output-format", "text",
		"--add-dir", scratchDir,
		"--allowedTools", "Read", "Edit", "Write", "MultiEdit", "Glob", "Grep",
		"--disallowedTools", "Bash", "WebFetch", "WebSearch",
	)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), r.Env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("claude binary %q not found: %w", r.Bin, err)
		}
		if r.Timeout > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("claude run exceeded the timeout of %s", r.Timeout)
		}
		return fmt.Errorf("run %s: %w", r.Bin, err)
	}
	return nil
}
