// Package breaks implements the break schedule without a desktop or real clock.
package breaks

import "time"

type Kind uint8

const (
	Short Kind = iota
	Long
)

func (k Kind) Label() string {
	if k == Long {
		return "Long break"
	}
	return "Short break"
}

type Phase uint8

const (
	Waiting Phase = iota
	Active
	Paused
	Sleeping
)

type State struct {
	Phase       Phase
	Kind        Kind
	Remaining   time.Duration
	Duration    time.Duration
	CanSkip     bool
	CanPostpone bool
}

// Scheduler is driven by a single caller supplying timestamps.
type Scheduler struct {
	settings   Settings
	activeRule Rule
	phase      Phase
	shorts     int
	deadline   time.Time
	started    time.Time
	noticeAt   time.Time
	postponed  bool
	sleeping   bool
}

func New(settings Settings, now time.Time) *Scheduler {
	s := &Scheduler{settings: settings}
	s.reset(now)
	return s
}

func (s *Scheduler) kind() Kind {
	if s.shorts == 2 {
		return Long
	}
	return Short
}

func (s *Scheduler) State(now time.Time) State {
	state := State{Phase: s.phase, Kind: s.kind()}
	if s.phase == Paused {
		return state
	}
	if s.sleeping {
		state.Phase = Sleeping
		return state
	}
	state.Remaining = max(0, s.deadline.Sub(now))
	if s.phase == Active {
		rule := s.activeRule
		state.Duration = rule.Duration
		state.CanSkip = !rule.Strict && state.Remaining > 0
		state.CanPostpone = state.CanSkip && rule.Postpone > 0 && !s.postponed && now.Sub(s.started) < rule.Duration*3/10
	}
	return state
}

// Advance applies at most one transition and reports a newly due advance notice.
// If the notice was missed, the break starts directly.
func (s *Scheduler) Advance(now time.Time) (notice bool) {
	if s.sleeping || s.phase == Paused {
		return false
	}
	if !now.Before(s.deadline) {
		if s.phase == Active {
			s.finish(now)
		} else {
			s.Start(now)
		}
		return false
	}
	if s.phase == Waiting && !s.noticeAt.IsZero() && !now.Before(s.noticeAt) {
		s.noticeAt = time.Time{}
		return true
	}
	return false
}

// Start takes the next scheduled break now, preserving its place in the cycle.
func (s *Scheduler) Start(now time.Time) bool {
	if s.sleeping || s.phase != Waiting {
		return false
	}
	s.phase = Active
	s.started = now
	s.activeRule = s.settings.Rule(s.kind())
	s.deadline = now.Add(s.activeRule.Duration)
	s.noticeAt = time.Time{}
	return true
}

func (s *Scheduler) Skip(now time.Time) bool {
	if !s.State(now).CanSkip {
		return false
	}
	s.finish(now)
	return true
}

func (s *Scheduler) Postpone(now time.Time) bool {
	if !s.State(now).CanPostpone {
		return false
	}
	s.wait(now, s.activeRule.Postpone)
	s.postponed = true
	return true
}

func (s *Scheduler) Pause() bool {
	if s.sleeping || s.phase == Paused || (s.phase == Active && s.activeRule.Strict) {
		return false
	}
	s.phase = Paused
	s.noticeAt = time.Time{}
	return true
}

func (s *Scheduler) Resume(now time.Time) bool {
	if s.phase != Paused || s.sleeping {
		return false
	}
	s.reset(now)
	return true
}

func (s *Scheduler) Suspend() { s.sleeping = true }

// Wake discards stale deadlines and preserves a manual pause.
func (s *Scheduler) Wake(now time.Time) {
	s.sleeping = false
	if s.phase != Paused {
		s.reset(now)
	}
}

// Reconfigure starts a fresh cycle when waiting. An active break keeps the rule
// it started with. A suspended or paused schedule uses the new settings when it
// resets on wake or resume. The caller supplies validated preferences.
func (s *Scheduler) Reconfigure(settings Settings, now time.Time) {
	if s.settings == settings {
		return
	}
	s.settings = settings
	if s.phase == Waiting && !s.sleeping {
		s.reset(now)
	}
}

func (s *Scheduler) reset(now time.Time) {
	s.shorts = 0
	s.wait(now, s.settings.Interval)
}

func (s *Scheduler) finish(now time.Time) {
	s.shorts = (s.shorts + 1) % 3
	s.wait(now, s.settings.Interval)
}

func (s *Scheduler) wait(now time.Time, delay time.Duration) {
	s.phase = Waiting
	s.deadline = now.Add(delay)
	s.noticeAt = time.Time{}
	if lead := s.settings.Rule(s.kind()).Notice; lead > 0 && lead < delay {
		s.noticeAt = s.deadline.Add(-lead)
	}
	s.postponed = false
}
