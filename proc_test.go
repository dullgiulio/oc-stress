package main

import (
	"bytes"
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

func TestOcStatusIsDesired(t *testing.T) {
	output := []byte(`NAME                     REVISION   DESIRED   CURRENT   TRIGGERED BY
test-shutdown-receiver   36         0         1         config,image(test-shutdown-receiver:latest)
`)
	testFn := ocStatusIsDesired("test-shutdown-receiver")
	err := testFn(bytes.NewReader(output))
	if err != errRunRetry {
		t.Errorf("got error '%v' instead of errRunRetry", err)
	}
	output = []byte(`NAME                     REVISION   DESIRED   CURRENT   TRIGGERED BY
test-shutdown-receiver   36         0         0         config,image(test-shutdown-receiver:latest)
`)
	err = testFn(bytes.NewReader(output))
	if err != nil {
		t.Errorf("got error '%v' instead of nil", err)
	}
	testFn = ocStatusIsDesired("test")
	err = testFn(bytes.NewReader(output))
	if err != errRunRetry {
		t.Errorf("got error '%v' instead of errRunRetry", err)
	}
}
