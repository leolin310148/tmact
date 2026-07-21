package statusd

import (
	"os"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	DefaultSocketPath      = "/tmp/tmact-statusd.sock"
	DefaultInterval        = 500 * time.Millisecond
	DefaultStaleAfter      = 10 * time.Second
	DefaultCaptureLines    = 40
	DefaultRunningDebounce = 5 * time.Second
	DefaultInitialSamples  = 2
	// DefaultPaneCols is the fixed tmux window width the daemon enforces so
	// scrollback history stays visually consistent across browsers and devices.
	// CSS pre-wrap in the web UI handles narrower viewports without re-flowing
	// tmux's grid (which would fragment the history at the old width).
	DefaultPaneCols = 140
	// DefaultPaneRows is the corresponding fixed window height.
	DefaultPaneRows = 40
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
	// PaneCols / PaneRows define the fixed tmux window size the daemon keeps
	// every detached window at. Zero disables the sweep entirely.
	PaneCols int
	PaneRows int

	// SessionSave periodically persists all local tmux sessions. SessionRestore
	// restores the last valid snapshot once at daemon startup, but only after a
	// successful capture proves tmux currently has zero sessions.
	SessionSave              bool
	SessionRestore           bool
	SessionSaveInterval      time.Duration
	SessionSnapshotRetention int
	SessionSnapshotDir       string

	// ClosedSessionsPath is where the daemon records recently closed local
	// sessions (for the web UI's reopen history). Empty keeps the log in
	// memory only; ClosedSessionsMax caps the retained entries.
	ClosedSessionsPath string
	ClosedSessionsMax  int

	// Peers is the list of remote statusd instances whose snapshots are
	// merged into the local one. Empty disables federation.
	Peers []Peer
	// CostPeers is the list of remote tmact instances that only contribute
	// token-spend from /api/agent-usage. They are not merged into snapshots.
	CostPeers    []Peer
	PeerInterval time.Duration
	PeerTimeout  time.Duration

	// UsageInterval / SpendInterval set the web agent-usage refresh cadences.
	// Quota (rate-limited provider endpoints) refreshes on UsageInterval;
	// token spend (local disk scan + peers) on SpendInterval. Zero uses the
	// web package defaults (5m / 60s).
	UsageInterval time.Duration
	SpendInterval time.Duration

	Now              func() time.Time
	Sleep            func(time.Duration)
	ListPanes        func() ([]tmux.Pane, error)
	CapturePane      func(string, int) (string, error)
	CapturePaneANSI  func(string, int) (string, error)
	SetSessionOption func(string, string, string) error
	// ListWindowSizes and ResizeWindow are injection points for the pane-width
	// sweep; default to the live tmux helpers.
	ListWindowSizes  func() ([]tmux.WindowSize, error)
	ResizeWindow     func(target string, cols, rows int) error
	ListSessionState func() ([]tmux.SessionStatePane, error)
	RestoreClient    tmux.RestoreClient
	HomeDir          func() (string, error)
	DirExists        func(string) bool
	Logf             func(format string, args ...any)
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
		c.CapturePaneANSI = tmux.CapturePaneANSI
	}
	if c.SetSessionOption == nil {
		c.SetSessionOption = tmux.SetSessionOption
	}
	if c.PaneCols < 0 {
		c.PaneCols = 0
	}
	if c.PaneRows < 0 {
		c.PaneRows = 0
	}
	if c.ListWindowSizes == nil {
		c.ListWindowSizes = tmux.ListWindowSizes
	}
	if c.ResizeWindow == nil {
		c.ResizeWindow = tmux.ResizeWindow
	}
	if c.SessionSaveInterval <= 0 {
		c.SessionSaveInterval = DefaultSessionSaveInterval
	}
	if c.SessionSnapshotRetention <= 0 {
		c.SessionSnapshotRetention = DefaultSessionSnapshotRetention
	}
	if c.SessionSnapshotDir == "" {
		c.SessionSnapshotDir = DefaultSessionSnapshotDir()
	}
	if c.ClosedSessionsPath == "" {
		c.ClosedSessionsPath = DefaultClosedSessionsPath()
	}
	if c.ClosedSessionsMax <= 0 {
		c.ClosedSessionsMax = DefaultClosedSessionsMax
	}
	if c.ListSessionState == nil {
		c.ListSessionState = tmux.ListSessionState
	}
	if c.RestoreClient == nil {
		c.RestoreClient = tmux.LiveRestoreClient{}
	}
	if c.HomeDir == nil {
		c.HomeDir = os.UserHomeDir
	}
	if c.DirExists == nil {
		c.DirExists = directoryExists
	}
	return c
}
