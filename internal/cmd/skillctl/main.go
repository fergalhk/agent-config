// Command skillctl keeps forked skills in step with their upstream sources.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fergalhk/agent-config/internal/config"
)

const usage = "usage: skillctl <check|list-forked> [--root dir]"

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
