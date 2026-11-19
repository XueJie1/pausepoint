package breaks

import (
	"fmt"
	"time"
)

const MaxDuration = 24 * time.Hour

// Settings holds the user-editable break schedule and behavior.
type Settings struct {
	Interval time.Duration
	Short    Rule
	Long     Rule
}

type Rule struct {
	Duration time.Duration
	Notice   time.Duration
	Postpone time.Duration
	Strict   bool
}

func Defaults() Settings {
	return Settings{
		Interval: 10 * time.Minute,
		Short:    Rule{Duration: 20 * time.Second, Notice: 10 * time.Second, Postpone: 2 * time.Minute},
		Long:     Rule{Duration: 5 * time.Minute, Notice: 30 * time.Second, Postpone: 5 * time.Minute},
	}
}

func (s Settings) Rule(kind Kind) Rule {
	if kind == Long {
		return s.Long
	}
	return s.Short
}

func (s Settings) Validate() error {
	if !wholeSecondsInRange(s.Interval, time.Second, MaxDuration) {
		return fmt.Errorf("work interval must be 1–86400 whole seconds")
	}
	for _, kind := range [...]Kind{Short, Long} {
		r := s.Rule(kind)
		if !wholeSecondsInRange(r.Duration, time.Second, MaxDuration) {
			return fmt.Errorf("%s duration must be 1–86400 whole seconds", kind.Label())
		}
		if !wholeSecondsInRange(r.Notice, 0, s.Interval-time.Second) {
			return fmt.Errorf("%s notice must be less than the work interval", kind.Label())
		}
		if !wholeSecondsInRange(r.Postpone, 0, MaxDuration) {
			return fmt.Errorf("%s postponement must be 0–86400 whole seconds", kind.Label())
		}
	}
	return nil
}

func wholeSecondsInRange(d, low, high time.Duration) bool {
	return d >= low && d <= high && d%time.Second == 0
}
