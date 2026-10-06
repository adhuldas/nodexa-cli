package manifest

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Healthcheck is a service's compose healthcheck, in the wire shape
// nodexa-registry and nodexa-backend take (their HealthCheck model): Test[0]
// picks the probe -- CMD / CMD-SHELL run inside the container, HTTP and TCP
// run from the device, NONE disables it -- and the times are whole seconds,
// 0 meaning "use the default" (30s interval, 10s timeout, 3 retries).
type Healthcheck struct {
	Test        []string `json:"test" yaml:"test"`
	Interval    int      `json:"interval,omitempty" yaml:"-"`
	Timeout     int      `json:"timeout,omitempty" yaml:"-"`
	Retries     int      `json:"retries,omitempty" yaml:"-"`
	StartPeriod int      `json:"start_period,omitempty" yaml:"-"`
}

// UnmarshalYAML reads compose's healthcheck block: `test` is a string
// (shorthand for CMD-SHELL) or a list; interval, timeout and start_period
// are durations ("30s", "1m30s"); `disable: true` is NONE.
func (h *Healthcheck) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Test        yaml.Node `yaml:"test"`
		Interval    string    `yaml:"interval"`
		Timeout     string    `yaml:"timeout"`
		Retries     int       `yaml:"retries"`
		StartPeriod string    `yaml:"start_period"`
		Disable     bool      `yaml:"disable"`
	}
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	if raw.Disable {
		*h = Healthcheck{Test: []string{"NONE"}}
		return nil
	}

	var test []string
	switch raw.Test.Kind {
	case yaml.ScalarNode:
		test = []string{"CMD-SHELL", raw.Test.Value}
	case yaml.SequenceNode:
		if err := raw.Test.Decode(&test); err != nil {
			return fmt.Errorf("healthcheck test: %w", err)
		}
	default:
		return fmt.Errorf("healthcheck: test is required (a string, or a list starting with CMD, CMD-SHELL, HTTP, TCP or NONE)")
	}

	var err error
	out := Healthcheck{Test: test, Retries: raw.Retries}
	if out.Interval, err = seconds("interval", raw.Interval); err != nil {
		return err
	}
	if out.Timeout, err = seconds("timeout", raw.Timeout); err != nil {
		return err
	}
	if out.StartPeriod, err = seconds("start_period", raw.StartPeriod); err != nil {
		return err
	}
	*h = out
	return nil
}

// seconds turns a compose duration into whole seconds, rounding up so a
// sub-second value doesn't become 0 ("use the default"). "" is 0.
func seconds(field, s string) (int, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("healthcheck %s %q: not a duration like 30s or 1m30s", field, s)
	}
	if d < 0 {
		return 0, fmt.Errorf("healthcheck %s %q: can't be negative", field, s)
	}
	return int(math.Ceil(d.Seconds())), nil
}

// Healthchecks returns each service's healthcheck keyed by service name,
// leaving out services without one.
func (m *Manifest) Healthchecks() map[string]*Healthcheck {
	out := map[string]*Healthcheck{}
	for _, s := range m.Services {
		if s.Healthcheck != nil {
			out[s.Name] = s.Healthcheck
		}
	}
	return out
}
