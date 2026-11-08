package claude

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const maxTitleRunes = 200

// Result is the pull request text that Claude Code writes.
type Result struct {
	Title string
	Body  string
}

// ParseResult reads a result file. Line 1 is the title. Line 2 is blank.
// The rest is the body.
func ParseResult(r io.Reader) (Result, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Result{}, fmt.Errorf("read result: %w", err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")

	title, rest, _ := strings.Cut(text, "\n")
	title = strings.TrimSpace(title)
	if title == "" {
		return Result{}, errors.New("result title is empty")
	}
	if n := utf8.RuneCountInString(title); n > maxTitleRunes {
		return Result{}, fmt.Errorf("result title has %d characters, the limit is %d", n, maxTitleRunes)
	}

	second, body, _ := strings.Cut(rest, "\n")
	if strings.TrimSpace(second) != "" {
		return Result{}, errors.New("result line 2 must be blank")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Result{}, errors.New("result body is empty")
	}
	return Result{Title: title, Body: body}, nil
}
