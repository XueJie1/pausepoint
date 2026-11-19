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

type Field uint8

const (
	Interval Field = iota
	ShortDuration
	ShortNotice
	ShortPostpone
	LongDuration
	LongNotice
	LongPostpone
)

// ValidationError identifies the invalid field without requiring callers to parse an English error.
type ValidationError struct{ Field Field }

func (e ValidationError) Error() string {
	labels := [...]string{"work interval", "short break duration", "short break notice", "short break postponement", "long break duration", "long break notice", "long break postponement"}
	label := labels[e.Field]
	switch e.Field {
	case ShortNotice, LongNotice:
		return label + " must be less than the work interval"
	case ShortPostpone, LongPostpone:
		return fmt.Sprintf("%s must be 0–86400 whole seconds", label)
	default:
		return fmt.Sprintf("%s must be 1–86400 whole seconds", label)
	}
}

func (s Settings) Validate() error {
	if !wholeSecondsInRange(s.Interval, time.Second, MaxDuration) {
		return ValidationError{Interval}
	}
	for _, fields := range [...]struct {
		rule                       Rule
		duration, notice, postpone Field
	}{
		{s.Short, ShortDuration, ShortNotice, ShortPostpone},
		{s.Long, LongDuration, LongNotice, LongPostpone},
	} {
		if !wholeSecondsInRange(fields.rule.Duration, time.Second, MaxDuration) {
			return ValidationError{fields.duration}
		}
		if !wholeSecondsInRange(fields.rule.Notice, 0, s.Interval-time.Second) {
			return ValidationError{fields.notice}
		}
		if !wholeSecondsInRange(fields.rule.Postpone, 0, MaxDuration) {
			return ValidationError{fields.postpone}
		}
	}
	return nil
}

func wholeSecondsInRange(d, low, high time.Duration) bool {
	return d >= low && d <= high && d%time.Second == 0
}
