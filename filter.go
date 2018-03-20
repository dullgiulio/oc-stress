package main

import (
	"bufio"
	"bytes"
	"io"
	"log"
)

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

type filter struct {
	match [][]byte
	sink  chan<- string
}

func newFilter(match []string, sink chan<- string) *filter {
	bms := make([][]byte, len(match))
	for i, m := range match {
		bms[i] = []byte(m)
	}
	return &filter{
		match: bms,
		sink:  sink,
	}
}

func (f *filter) matches(bs []byte) bool {
	if len(f.match) == 0 {
		return true // empty filter is catch all
	}
	for i := range f.match {
		if bytes.Contains(bs, f.match[i]) {
			return true // ANY of the matchable strings
		}
	}
	return false
}

func (f *filter) push(s string) {
	f.sink <- s
}

func (f *filter) close() {
	close(f.sink)
}

func slurp(r io.Reader, filters []*filter) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		for i := range filters {
			log.Printf("[debug] input: %s", sc.Text())
			if filters[i].matches(sc.Bytes()) {
				filters[i].push(sc.Text())
			}
		}
	}
	for i := range filters {
		filters[i].close()
	}
	return sc.Err()
}
