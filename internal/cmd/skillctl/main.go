// Command skillctl keeps forked skills in step with their upstream sources.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fergalhk/agent-config/internal/claude"
	"github.com/fergalhk/agent-config/internal/config"
	"github.com/fergalhk/agent-config/internal/discover"
	"github.com/fergalhk/agent-config/internal/github"
	"github.com/fergalhk/agent-config/internal/gitx"
	"github.com/fergalhk/agent-config/internal/update"
)

const usage = "usage: skillctl <check|list-forked|discover|update|skill-for-pr> [flags]"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(check(os.Args[2:]))
	case "list-forked":
		os.Exit(listForked(os.Args[2:]))
	case "discover":
		os.Exit(discoverCmd(os.Args[2:]))
	case "update":
		os.Exit(updateCmd(os.Args[2:]))
	case "skill-for-pr":
		os.Exit(skillForPR(os.Args[2:]))
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
}

// loadSkills parses the flags and loads the skills file under the repository root.
func loadSkills(name string, args []string) (string, config.Skills, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	skills, err := config.LoadSkills(filepath.Join(*root, config.SkillsPath))
	return *root, skills, err
}

func check(args []string) int {
	root, skills, err := loadSkills("check", args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	findings := config.Validate(root, skills)
	for _, f := range findings {
		fmt.Println(f)
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

func listForked(args []string) int {
	_, skills, err := loadSkills("list-forked", args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	out, err := json.Marshal(skills.Forked())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(string(out))
	return 0
}

func discoverCmd(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	api, err := github.NewFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	scratch, err := os.MkdirTemp("", "skillctl-discover-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer os.RemoveAll(scratch)

	err = discover.Run(context.Background(), discover.Deps{
		API:       api,
		Repo:      gitx.Open(*root),
		Root:      *root,
		Scratch:   scratch,
		RunChecks: RunChecks,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func updateCmd(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	skill := fs.String("skill", "", "name of the forked skill to update")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *skill == "" {
		fmt.Fprintln(os.Stderr, "update: --skill is required")
		return 2
	}
	api, err := github.NewFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	scratch, err := os.MkdirTemp("", "skillctl-update-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer os.RemoveAll(scratch)

	err = update.Run(context.Background(), update.Deps{
		API: api,
		Git: gitx.Open(*root),
		Upstream: func(ctx context.Context, url string) (update.UpstreamRepo, error) {
			repo, err := gitx.Clone(ctx, url, filepath.Join(scratch, "upstream"))
			if err != nil {
				return nil, err
			}
			return repo, nil
		},
		Claude:    claude.Runner{Bin: "claude", Timeout: 30 * time.Minute},
		Root:      *root,
		Scratch:   scratch,
		RunChecks: RunChecks,
		Now:       time.Now,
	}, *skill)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// skillForPR prints the skill name for an update PR. It prints nothing for other PRs.
func skillForPR(args []string) int {
	fs := flag.NewFlagSet("skill-for-pr", flag.ContinueOnError)
	number := fs.Int("pr", 0, "pull request number")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *number <= 0 {
		fmt.Fprintln(os.Stderr, "skill-for-pr: --pr is required")
		return 2
	}
	api, err := github.NewFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	name, ok, err := update.SkillForPR(context.Background(), api, *number)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if ok {
		fmt.Println(name)
	}
	return 0
}

// RunChecks validates the skills file against the tree under root.
// Then it runs the Go tests and passes their output through.
func RunChecks(root string) error {
	skills, err := config.LoadSkills(filepath.Join(root, config.SkillsPath))
	if err != nil {
		return err
	}
	if findings := config.Validate(root, skills); len(findings) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(findings, "; "))
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test: %w", err)
	}
	return nil
}
