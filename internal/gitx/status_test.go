package gitx

import (
	"reflect"
	"testing"
)

func TestParseStatusPorcelain(t *testing.T) {
	in := " M skills/a/SKILL.md\x00?? docs/agents/skills/a.md\x00R  new.md\x00old.md\x00"
	want := []string{"skills/a/SKILL.md", "docs/agents/skills/a.md", "new.md"}
	if got := ParseStatusPorcelain(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := ParseStatusPorcelain(""); got != nil {
		t.Fatalf("empty input: got %v, want nil", got)
	}
}
