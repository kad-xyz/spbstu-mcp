package main

import (
	"testing"

	"github.com/kad-xyz/spbstu-mcp/internal/ruz"
)

func TestPick(t *testing.T) {
	groups := []ruz.Match{{ID: 1, Name: "5130902/30201"}, {ID: 2, Name: "з5130902/30201"}}
	if m, many := pick(groups, "З5130902/30201"); many != nil || m.ID != 2 {
		t.Errorf("exact match ignoring case: got %v %v", m, many)
	}
	if _, many := pick(groups, "5130902"); many == nil || len(many.Matches) != 2 {
		t.Error("several matches must be returned to choose from")
	}
	if m, many := pick(groups[:1], "5130902"); many != nil || m.ID != 1 {
		t.Error("a single match must be taken")
	}
	if _, many := pick(nil, "x"); many == nil || many.Note != "Nothing found." {
		t.Error("no match must say so")
	}
}
