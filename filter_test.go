package main

import (
	"testing"
)

func TestSplitOutputLine(t *testing.T) {
	line := []byte("test-shutdown-receiver   36         0         1         config,image(test-shutdown-receiver:latest)")
	expected := []string{"test-shutdown-receiver", "36", "0", "1", "config,image(test-shutdown-receiver:latest)"}
	words := split(line)
	if len(words) != len(expected) {
		t.Fatalf("expected %d words, got %d (%+v)", len(expected), len(words), words)
	}
	for i := range words {
		if words[i] != expected[i] {
			t.Errorf("word %d is %q, expected '%s'", i, words[i], expected[i])
		}
	}
}
