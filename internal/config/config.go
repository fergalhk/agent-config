// Package config loads and validates the skillctl configuration files.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// SkillsPath is the file that lists every skill in the repository.
	SkillsPath = "internal/config/skills.yaml"
	// ForkSourcesPath is the file that lists the upstream sources to discover skills from.
	ForkSourcesPath = "internal/config/skill-forks.yaml"
)

const (
	typeForked     = "forked"
	typeFirstParty = "first_party"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Source locates a skill directory in an upstream repository.
type Source struct {
	Repo      string `yaml:"repo"`
	Directory string `yaml:"directory"`
}

// Fork describes where a forked skill comes from.
type Fork struct {
	Source Source `yaml:"source"`
}

// Skill is one entry in skills.yaml. Type is "forked" or "first_party".
type Skill struct {
	Type string `yaml:"type"`
	Fork *Fork  `yaml:"fork"`
}

// Skills maps a skill name to its entry.
type Skills map[string]Skill

// Forked returns the sorted names of the skills with type "forked".
func (s Skills) Forked() []string {
	names := []string{}
	for name, skill := range s {
		if skill.Type == typeForked {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ForkSource is one entry in skill-forks.yaml.
type ForkSource struct {
	Source struct {
		Repository string `yaml:"repository"`
		Directory  string `yaml:"directory"`
	} `yaml:"source"`
	Prefix string   `yaml:"prefix"`
	Ignore []string `yaml:"ignore"`
}

// LoadSkills reads the skills file at path.
func LoadSkills(path string) (Skills, error) {
	skills := Skills{}
	if err := decodeFile(path, &skills); err != nil {
		return nil, err
	}
	return skills, nil
}

// LoadForkSources reads the fork sources file at path.
func LoadForkSources(path string) ([]ForkSource, error) {
	var sources []ForkSource
	if err := decodeFile(path, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// decodeFile decodes a YAML file and rejects unknown keys. An empty file is valid.
func decodeFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// ShaPath returns the path of the file that records the last merged upstream commit.
func ShaPath(name string) string {
	return "internal/forked-skills/" + name + "/last-commit-sha"
}

// ReadSha reads the recorded commit for a skill. The value must be 40 hex characters.
func ReadSha(root, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ShaPath(name)))
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(data))
	if !shaPattern.MatchString(sha) {
		return "", fmt.Errorf("%s does not contain a 40 character hex commit", ShaPath(name))
	}
	return sha, nil
}
