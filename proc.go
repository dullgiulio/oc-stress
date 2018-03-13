package main

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
)

type mstatus struct {
	nlines  int64
	matches int64
	err     error
}

type matcher struct {
	reader io.Reader
	match  [][]byte
	update chan *status
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
		update: make(chan *status),
	}
}

func (m *matcher) run() {
	for st := range m.update {
		m.status.nlines += st.nlines
		m.status.matches += st.matches
		m.status.err = st.err
		if st.err != nil {
			log.Printf("[error] %v", err) // TODO: context etc
		}
	}
}

func (m *matcher) matches(bs []byte) bool {
	for i := range m.match {
		if bytes.Contains(bs, m.match[i]) {
			return true // ANY of the matchable strings
		}
	}
	return false
}

func (m *matcher) slurp(updateEvery int) {
	sc := bufio.NewScanner(m.reader)
	st := &mstatus{}
	for scanner.Scan() {
		if m.matches(scanner.Bytes()) {
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
	if err := scanner.Err(); err != nil {
		st.err = err
	}
	m.update <- st
}
