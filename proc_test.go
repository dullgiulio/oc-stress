package main

import (
	"bytes"
	"testing"
)

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
