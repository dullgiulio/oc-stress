package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

var errRunRetry = errors.New("retry")

type retryFn func(r io.Reader) error

func ocScale(dc string, replicas int) error {
	if output, err := exec.Command("oc", "scale", fmt.Sprintf("--replicas=%d", replicas), fmt.Sprintf("dc/%s", dc)).CombinedOutput(); err != nil {
		return fmt.Errorf("could not run oc scale to %d for dc %s: %v: %q", replicas, dc, err, string(output))
	}
	if err := runUntil(exec.Command("oc", "get", fmt.Sprintf("dc/%s", dc)), ocStatusIsDesired(dc), 1*time.Second); err != nil {
		return fmt.Errorf("could not satisfy scaling change to dc: %v", err)
	}
	return nil
}

func ocLogs(dc string, m *matcher, errs chan<- error) {
	cmd := exec.Command("oc", "logs", "-f", fmt.Sprintf("dc/%s", dc))
	var (
		out  bytes.Buffer
		oerr bytes.Buffer
	)
	cmd.Stdout = &out
	cmd.Stderr = &oerr
	done := make(chan struct{})
	go m.run(done, errs)
	go m.slurp(&out, 3) // TODO: Number of lines between updates, move up
	var failed bool
	if err := cmd.Start(); err != nil {
		errs <- fmt.Errorf("cannot start oc logs command on %s: %v", dc, err)
		failed = true
	}
	<-done
	if !failed {
		if err := cmd.Wait(); err != nil {
			errs <- fmt.Errorf("error running oc logs on %s: %v", dc, err)
		}
	}
	close(errs)
}

func ocStatusIsDesired(pod string) retryFn {
	return func(r io.Reader) error {
		sc := bufio.NewScanner(r)
		sc.Scan()
		if err := sc.Err(); err != nil {
			return fmt.Errorf("cannot read command output: %v", err)
		}
		for sc.Scan() {
			fields := split(sc.Bytes())
			if len(fields) != 5 {
				return fmt.Errorf("expected 5 fields in line %q", sc.Text())
			}
			if fields[0] != pod {
				continue
			}
			if fields[2] != fields[3] {
				return errRunRetry
			}
			return nil
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("cannot read command output: %v", err)
		}
		return errRunRetry
	}
}

func split(line []byte) []string {
	var i, j int
	words := make([]string, 0)
	for ; i < len(line); i++ {
		if !isspace(line[i]) {
			continue
		}
		if i <= j {
			continue
		}
		words = append(words, string(line[j:i]))
		for ; i < len(line); i++ {
			if !isspace(line[i]) {
				break
			}
		}
		j = i
	}
	if j < i {
		words = append(words, string(line[j:i]))
	}
	return words
}

func isspace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\v' || b == '\n'
}

func runUntil(cmd *exec.Cmd, fn retryFn, sleep time.Duration) error {
	for {
		out, err := cmd.Output()
		if err != nil {
			return err
		}
		err = fn(bytes.NewReader(out))
		if err == nil {
			return nil
		}
		if err != errRunRetry {
			return err
		}
		time.Sleep(sleep)
	}
	return nil
}

type mstatus struct {
	nlines  int64
	matches int64
	err     error
}

type matcher struct {
	match  [][]byte
	update chan *mstatus
	status *mstatus
}

func newMatcher(match []string) *matcher {
	bms := make([][]byte, len(match))
	for i, m := range match {
		bms[i] = []byte(m)
	}
	return &matcher{
		match:  bms,
		status: &mstatus{},
		update: make(chan *mstatus),
	}
}

func (m *matcher) run(done chan struct{}, errs chan<- error) {
	for st := range m.update {
		m.status.nlines += st.nlines
		m.status.matches += st.matches
		m.status.err = st.err
		if st.err != nil {
			errs <- st.err
		}
	}
	close(done)
}

func (m *matcher) matches(bs []byte) bool {
	for i := range m.match {
		if bytes.Contains(bs, m.match[i]) {
			return true // ANY of the matchable strings
		}
	}
	return false
}

func (m *matcher) slurp(r io.Reader, updateEvery int64) {
	sc := bufio.NewScanner(r)
	st := &mstatus{}
	for sc.Scan() {
		if m.matches(sc.Bytes()) {
			st.matches++
		}
		st.nlines++
		// TODO: this depends on lines, not time...
		// 		 maybe wrap the reader into some sort of timed reader
		if st.nlines%updateEvery == 0 {
			m.update <- st
			st = &mstatus{}
		}
	}
	if err := sc.Err(); err != nil {
		st.err = err
	}
	m.update <- st
	close(m.update)
}
