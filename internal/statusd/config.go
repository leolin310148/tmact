package statusd

import (
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	DefaultSocketPath      = "/tmp/tmact-statusd.sock"
	DefaultInterval        = time.Second
	DefaultStaleAfter      = 10 * time.Second
	DefaultCaptureLines    = 40
	DefaultRunningDebounce = 5 * time.Second
	DefaultInitialSamples  = 2
)

type Config struct {
	Interval           time.Duration
	SocketPath         string
	LogPath            string
	TmuxOptions        bool
	CaptureLines       int
	InitialSamples     int
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
	if c.SocketPath == "" {
		c.SocketPath = DefaultSocketPath
	}
	if c.CaptureLines <= 0 {
		c.CaptureLines = DefaultCaptureLines
	}
	if c.InitialSamples <= 0 {
		c.InitialSamples = DefaultInitialSamples
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
