package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/dullgiulio/oc-stress/jsoncomments"
)

type options struct {
	RateSecond int
	CrashMatch string
	LostMatch  string
}

type actionOpts map[string]interface{}

func (a actionOpts) getString(k string) (string, error) {
	v, ok := a[k]
	if !ok {
		return "", fmt.Errorf("key '%s' not found", k)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("key '%s' has value '%+v' which is not a string", k, v)
	}
	return s, nil
}

func (a actionOpts) getDuration(k string) (time.Duration, error) {
	var (
		d   time.Duration
		err error
	)
	v, ok := a[k]
	if !ok {
		return d, fmt.Errorf("key '%s' not found", k)
	}
	s, ok := v.(string)
	if !ok {
		return d, fmt.Errorf("key '%s' has value '%+v' which is not a duration string", k, v)
	}
	d, err = time.ParseDuration(s)
	if err != nil {
		return d, fmt.Errorf("key '%s' has value '%+v' which is not a duration: %v", k, v, err)
	}
	return d, nil
}

func (a actionOpts) getInt(k string) (int, error) {
	v, ok := a[k]
	if !ok {
		return 0, fmt.Errorf("key '%s' not found", k)
	}
	if s, ok := v.(string); ok {
		i, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("key '%s' has value '%+v' which is not an integer: %v", k, v, err)
		}
		return int(i), nil
	}
	if i, ok := v.(int); ok {
		return i, nil
	}
	if i, ok := v.(int32); ok {
		return int(i), nil
	}
	if i, ok := v.(int64); ok {
		return int(i), nil
	}
	return 0, fmt.Errorf("key '%s' has value '%+v' which is not an integer", k, v)
}

type config struct {
	Images  map[string]string
	Tests   map[string][]actionOpts
	Options options
}

func newConfig() *config {
	return &config{
		Images:  make(map[string]string),
		Tests:   make(map[string][]actionOpts),
		Options: options{},
	}
}

type action interface {
	init(opts actionOpts) error
	run() error
}

type scaleAction struct {
	pod          string
	wantedUnits  int
	currentUnits int
}

func (s *scaleAction) init(opts actionOpts) error {
	var err error
	s.pod, err = opts.getString("Pod")
	if err != nil {
		return fmt.Errorf("invalid 'Pod' option in 'scale' action: %v", err)
	}
	s.wantedUnits, err = opts.getInt("Units")
	if err != nil {
		return fmt.Errorf("invalid 'Units' option in 'scale' action: %v", err)
	}
	// TODO: determine current units via oc command
	return nil
}

func (s *scaleAction) run() error {
	// TODO
	fmt.Printf("running scale of pod %s to %d units\n", s.pod, s.wantedUnits)
	return nil
}

type pauseAction struct {
	duration time.Duration
}

func (p *pauseAction) init(opts actionOpts) error {
	var err error
	p.duration, err = opts.getDuration("For")
	if err != nil {
		return fmt.Errorf("invalid 'For' option in 'pause' action: %v", err)
	}
	return nil
}

func (p *pauseAction) run() error {
	// TODO
	fmt.Printf("running pause for %v\n", p.duration)
	return nil
}

func buildAction(opts actionOpts) (action, error) {
	var a action
	name, err := opts.getString("Action")
	if err != nil {
		return nil, fmt.Errorf("invalid action without 'Action' key: %v", err)
	}
	switch name {
	case "scale":
		a = &scaleAction{}
	case "pause":
		a = &pauseAction{}
	default:
		return nil, fmt.Errorf("invalid action type %s", name)
	}
	if err = a.init(opts); err != nil {
		return nil, fmt.Errorf("cannot init action %s: %v", name, err)
	}
	return a, nil
}

func buildTests(testscf map[string][]actionOpts) (map[string][]action, error) {
	tests := make(map[string][]action)
	for name, acts := range testscf {
		actions := make([]action, len(acts))
		for i, opts := range acts {
			a, err := buildAction(opts)
			if err != nil {
				return nil, fmt.Errorf("error in test '%s': %v", name, err)
			}
			actions[i] = a
		}
		tests[name] = actions
	}
	return tests, nil
}

func main() {
	flag.Parse()
	cfile := flag.Arg(0)
	if cfile == "" {
		log.Fatal("usage: oc-stress [options] config-file.json")
	}
	fh, err := os.Open(cfile)
	if err != nil {
		log.Fatalf("cannot open configuration file %s: %v", cfile, err)
	}
	jcr := jsoncomments.NewReader(fh)
	dec := json.NewDecoder(jcr)
	config := newConfig()
	if err := dec.Decode(&config); err != nil {
		log.Fatalf("cannot decode JSON configuration from %s: %v", cfile, err)
	}
	tests, err := buildTests(config.Tests)
	config.Tests = nil
	for name, actions := range tests {
		fmt.Printf("[start] %s\n", name)
		for i := range actions {
			if err := actions[i].run(); err != nil {
				log.Fatal("error while running action: %v", err)
				// TODO: run finalizers
			}
		}
		fmt.Printf("[end] %s\n", name)
	}
}
