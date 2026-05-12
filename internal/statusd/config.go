package statusd

import (
	"time"

	"tmact/internal/tmux"
)

const (
	DefaultStatePath       = "/tmp/tmact-status.json"
	DefaultInterval        = time.Second
	DefaultStaleAfter      = 10 * time.Second
	DefaultCaptureLines    = 120
	DefaultRunningDebounce = 5 * time.Second
)

type Config struct {
	Interval           time.Duration
	StatePath          string
	LogPath            string
	TmuxOptions        bool
	CaptureLines       int
	RunningDebounce    time.Duration
	StaleAfter         time.Duration
	IdleIgnorePatterns []string
	IncludeSessions    []string
	ExcludeSessions    []string

	Now              func() time.Time
	Sleep            func(time.Duration)
	ListPanes        func() ([]tmux.Pane, error)
	CapturePane      func(string, int) (string, error)
	SetSessionOption func(string, string, string) error
}

func (c Config) withDefaults() Config {
	if c.Interval <= 0 {
		c.Interval = DefaultInterval
	}
	if c.StatePath == "" {
		c.StatePath = DefaultStatePath
	}
	if c.CaptureLines <= 0 {
		c.CaptureLines = DefaultCaptureLines
	}
	if c.RunningDebounce <= 0 {
		c.RunningDebounce = DefaultRunningDebounce
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = DefaultStaleAfter
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Sleep == nil {
		c.Sleep = time.Sleep
	}
	if c.ListPanes == nil {
		c.ListPanes = tmux.ListAllPanes
	}
	if c.CapturePane == nil {
		c.CapturePane = tmux.CapturePane
	}
	if c.SetSessionOption == nil {
		c.SetSessionOption = tmux.SetSessionOption
	}
	return c
}
