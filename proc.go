package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

var errRunRetry = errors.New("retry")

type retryFn func(r io.Reader) error

type retryCmd struct {
	verify retryFn
	cmd    *exec.Cmd
}

func (r *retryCmd) run(max int, sleep time.Duration) error {
	for i := 0; i < max; i++ {
		out, err := r.cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not run command: %v: %q", err, out)
		}
		err = r.verify(bytes.NewReader(out))
		if err == nil {
			return nil
		}
		if err != errRunRetry {
			return fmt.Errorf("could not verify command output: %v", err)
		}
		time.Sleep(sleep)
	}
	return fmt.Errorf("could not reach expected result after %d retries", max)
}

func ocScale(dc string, replicas int) error {
	if output, err := exec.Command("oc", "scale", fmt.Sprintf("--replicas=%d", replicas), fmt.Sprintf("dc/%s", dc)).CombinedOutput(); err != nil {
		return fmt.Errorf("could not run oc scale to %d for dc %s: %v: %q", replicas, dc, err, string(output))
	}
	retry := &retryCmd{
		cmd:    exec.Command("oc", "get", fmt.Sprintf("dc/%s", dc)),
		verify: ocStatusIsDesired(dc),
	}
	retryMax := 10
	if replicas > 0 {
		retryMax = replicas * 10
	}
	if err := retry.run(retryMax, 1*time.Second); err != nil {
		return fmt.Errorf("could not satisfy scaling change to dc: %v", err)
	}
	return nil
}

func ocLogs(dc string, errs chan<- error) error {
	cmd := exec.Command("oc", "logs", "-f", fmt.Sprintf("dc/%s", dc))
	var (
		out    bytes.Buffer
		oerr   bytes.Buffer
		failed bool
	)
	cmd.Stdout = &out
	cmd.Stderr = &oerr
	lines := make(chan string, 20)
	all := newFilter(nil, lines)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for line := range lines {
			log.Printf("[output] %s", line)
		}
		wg.Done()
	}()
	go func() {
		if err := slurp(&out, []*filter{all}); err != nil {
			errs <- err
		}
	}()
	if err := cmd.Start(); err != nil {
		errs <- fmt.Errorf("cannot start oc logs command on %s: %v", dc, err)
		failed = true
	}
	log.Printf("[debug] log command started on %s", dc)
	wg.Wait()
	log.Printf("[debug] waited for output to be printed for %s logs", dc)
	if !failed {
		if err := cmd.Wait(); err != nil {
			errs <- fmt.Errorf("error running oc logs on %s: %v: %q", dc, err, oerr.String())
			return err
		}
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
