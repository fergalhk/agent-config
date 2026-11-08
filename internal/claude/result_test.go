package claude

import (
	"strings"
	"testing"
)

func TestParseResult_Valid(t *testing.T) {
	in := "Apply upstream change X\n\n## What changed upstream\nStuff\n"
	got, err := ParseResult(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Apply upstream change X" {
		t.Errorf("title = %q", got.Title)
	}
	if !strings.HasPrefix(got.Body, "## What changed upstream") {
		t.Errorf("body = %q", got.Body)
	}
}

func TestParseResult_EmptyTitle(t *testing.T) {
	if _, err := ParseResult(strings.NewReader("\n\nbody")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseResult_NoBody(t *testing.T) {
	if _, err := ParseResult(strings.NewReader("Title only\n")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseResult_CRLF(t *testing.T) {
	got, err := ParseResult(strings.NewReader("Title\r\n\r\nBody\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Title" || got.Body != "Body" {
		t.Errorf("got %+v", got)
	}
}

func TestParseResult_LongTitle(t *testing.T) {
	in := strings.Repeat("a", 201) + "\n\nBody\n"
	if _, err := ParseResult(strings.NewReader(in)); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseResult_NoBlankSecondLine(t *testing.T) {
	if _, err := ParseResult(strings.NewReader("Title\nBody\n")); err == nil {
		t.Fatal("expected error")
	}
}
