package cmd

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func strPtr(s string) *string {
	return &s
}

func TestGetValue(t *testing.T) {
	if got := getValue(nil); got != "" {
		t.Errorf("getValue(nil) = %q, want empty", got)
	}
	s := "hello"
	if got := getValue(&s); got != "hello" {
		t.Errorf("getValue(&hello) = %q, want hello", got)
	}
	empty := ""
	if got := getValue(&empty); got != "" {
		t.Errorf("getValue(&empty) = %q, want empty", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("shorter: got %q", got)
	}
	if got := truncate("abcde", 5); got != "abcde" {
		t.Errorf("exact: got %q", got)
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("empty: got %q", got)
	}
}

func TestContainsWithCase(t *testing.T) {
	if !containsWithCase("Hello World", "Hello", false) {
		t.Error("expected true for exact match")
	}
	if containsWithCase("Hello World", "hello", false) {
		t.Error("expected false for case-sensitive mismatch")
	}
	if !containsWithCase("Hello World", "hello", true) {
		t.Error("expected true for case-insensitive match")
	}
	if containsWithCase("Hello World", "xyz", false) {
		t.Error("expected false for no match")
	}
	if !containsWithCase("Hello", "", false) {
		t.Error("expected true for empty needle")
	}
	if containsWithCase("", "x", false) {
		t.Error("expected false for empty haystack")
	}
}

func TestEqualsWithCase(t *testing.T) {
	if !equalsWithCase("abc", "abc", false) {
		t.Error("expected true for equal strings")
	}
	if equalsWithCase("abc", "def", false) {
		t.Error("expected false for different strings")
	}
	if equalsWithCase("Abc", "abc", false) {
		t.Error("expected false for case-sensitive mismatch")
	}
	if !equalsWithCase("Abc", "abc", true) {
		t.Error("expected true for case-insensitive match")
	}
}

func TestBuildStatusFilters_All(t *testing.T) {
	filters := buildStatusFilters(true, false, false, false)
	if filters != nil {
		t.Errorf("--all should return nil, got %v", filters)
	}
}

func TestBuildStatusFilters_Default(t *testing.T) {
	filters := buildStatusFilters(false, false, false, false)
	if len(filters) == 0 {
		t.Error("default should return non-empty filters")
	}
	for _, f := range filters {
		if f == types.StackStatusDeleteComplete {
			t.Error("default should not include DELETE_COMPLETE")
		}
	}
}

func TestBuildStatusFilters_Complete(t *testing.T) {
	filters := buildStatusFilters(false, true, false, false)
	found := false
	for _, f := range filters {
		if f == types.StackStatusDeleteComplete {
			found = true
		}
	}
	if !found {
		t.Error("--complete should include DELETE_COMPLETE")
	}
}

func TestBuildStatusFilters_Deleted(t *testing.T) {
	filters := buildStatusFilters(false, false, true, false)
	found := false
	for _, f := range filters {
		if f == types.StackStatusDeleteComplete {
			found = true
		}
	}
	if !found {
		t.Error("--deleted should include DELETE_COMPLETE")
	}
}

func TestBuildStatusFilters_InProgress(t *testing.T) {
	filters := buildStatusFilters(false, false, false, true)
	found := false
	for _, f := range filters {
		if f == types.StackStatusCreateInProgress {
			found = true
		}
	}
	if !found {
		t.Error("--in-progress should include CREATE_IN_PROGRESS")
	}
}
