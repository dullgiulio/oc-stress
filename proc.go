package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"
)

var errRunRetry = errors.New("retry")

type retryFn func(r io.Reader) error

func ocScale(dc string, replicas int) error {
	if err := exec.Command("oc", "scale", fmt.Sprintf("--replicas=%d", replicas), dc).Run(); err != nil {
		return fmt.Errorf("could not run oc scale to %d for dc %s: %v", replicas, dc, err)
	}
	if err := runUntil(exec.Command("oc", "get", fmt.Sprintf("dc/%s", dc)), ocStatusIsDesired(dc), 1 second); err != nil {
		// TODO: error here should convey that we want to run the finalizer (scale to zero)
		return fmt.Errorf("could not satisfy scaling change to dc: %v", err)
	}
	return nil
}

func ocLogs(dc string, m *matcher) error {
	cmd := exec.Command("oc", "logs", "-f", fmt.Sprintf("dc/%s", dc))
	var (
		out bufio.Buffer
		oerr bufio.Buffer
	)
	cmd.Stdout = &out
	cmd.Stderr = &oerr
	m.reader = &out
	done := make(chan struct{})
	go m.run(done)
	go m.slurp(3) // TODO: Number of lines between updates, move up
	cmd.Start()
	<-done
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("error running oc logs on %s: %v", dc, err)
	}
	return nil
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
	reader io.Reader
	match  [][]byte
	update chan *mstatus
	status *mstatus
}

func newMatcher(r io.Reader, match []string) *matcher {
	bms := make([][]byte, len(match))
	for i, m := range match {
		bms[i] = []byte(m)
	}
	return &matcher{
		reader: r,
		match:  bms,
		update: make(chan *mstatus),
	}
}

func (m *matcher) run(done chan struct{}) {
	for st := range m.update {
		m.status.nlines += st.nlines
		m.status.matches += st.matches
		m.status.err = st.err
		if st.err != nil {
			log.Printf("[error] %v", st.err) // TODO: context etc
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

func (m *matcher) slurp(updateEvery int64) {
	sc := bufio.NewScanner(m.reader)
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
