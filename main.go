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
		return "", fmt.Errorf("key %q not found", k)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("key %q has value '%+v' which is not a string", k, v)
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
		return d, fmt.Errorf("key %q not found", k)
	}
	s, ok := v.(string)
	if !ok {
		return d, fmt.Errorf("key %q has value '%+v' which is not a duration string", k, v)
	}
	d, err = time.ParseDuration(s)
	if err != nil {
		return d, fmt.Errorf("key %q has value '%+v' which is not a duration: %v", k, v, err)
	}
	return d, nil
}

func (a actionOpts) getInt(k string) (int, error) {
	v, ok := a[k]
	if !ok {
		return 0, fmt.Errorf("key %q not found", k)
	}
	if s, ok := v.(string); ok {
		i, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("key %q has value '%+v' which is not an integer: %v", k, v, err)
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
	return 0, fmt.Errorf("key %q has value '%+v' which is not an integer", k, v)
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
	init(opts actionOpts, images map[string]string) error
	run() error
}

type scaleAction struct {
	pod   string
	units int
}

func (s *scaleAction) init(opts actionOpts, images map[string]string) error {
	var (
		ok  bool
		err error
	)
	s.pod, err = opts.getString("Pod")
	if err != nil {
		return fmt.Errorf("invalid 'Pod' option in 'scale' action: %v", err)
	}
	if s.pod, ok = images[s.pod]; !ok {
		return fmt.Errorf("invalid 'Pod' option in 'scale' action: image %s not defined in Images section", s.pod)
	}
	s.units, err = opts.getInt("Units")
	if err != nil {
		return fmt.Errorf("invalid 'Units' option in 'scale' action: %v", err)
	}
	return nil
}

func (s *scaleAction) run() error {
	log.Printf("[step] scaling %s to %d units", s.pod, s.units)
	if err := ocScale(s.pod, s.units); err != nil {
		return fmt.Errorf("cannot run scale step: %v", err)
	}
	return nil
}

type pauseAction struct {
	duration time.Duration
}

func (p *pauseAction) init(opts actionOpts, images map[string]string) error {
	var err error
	p.duration, err = opts.getDuration("For")
	if err != nil {
		return fmt.Errorf("invalid 'For' option in 'pause' action: %v", err)
	}
	return nil
}

func (p *pauseAction) run() error {
	log.Printf("[step] sleeping for %s", &p.duration)
	time.Sleep(p.duration)
	return nil
}

func buildAction(opts actionOpts, images map[string]string) (action, error) {
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
	if err = a.init(opts, images); err != nil {
		return nil, fmt.Errorf("cannot init action %s: %v", name, err)
	}
	return a, nil
}

func buildTests(testscf map[string][]actionOpts, images map[string]string) (map[string][]action, error) {
	tests := make(map[string][]action)
	for name, acts := range testscf {
		actions := make([]action, len(acts))
		for i, opts := range acts {
			a, err := buildAction(opts, images)
			if err != nil {
				return nil, fmt.Errorf("error in test %q: %v", name, err)
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
	tests, err := buildTests(config.Tests, config.Images)
	config.Tests = nil
	errs := make(chan error, 10)
	go ocLogs(config.Images["sender"], newMatcher([]string{config.Options.LostMatch}), errs)
	go func(pod string) {
		for err := range errs {
			log.Printf("[logs] %s: error: %s", pod, err)
		}
	}(config.Images["sender"])
	for name, actions := range tests {
		log.Printf("[step] start %s\n", name)
		for i := range actions {
			if err := actions[i].run(); err != nil {
				log.Printf("[step] error while running action: %v", err)
			}
		}
		log.Printf("[step] end %s\n", name)
	}
}
