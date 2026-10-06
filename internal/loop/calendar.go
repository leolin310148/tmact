package loop

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	// Embed the IANA database so calendar zones never depend on the host's
	// zoneinfo installation.
	_ "time/tzdata"

	"gopkg.in/yaml.v3"
)

// CalendarConfig restricts when a loop may start actions and flows. It is a
// loop-wide gate evaluated in an explicit IANA timezone: an action or flow may
// start only on a listed weekday inside one of the daily windows. Outside the
// calendar nothing is sent and no run/action counts are consumed; a schedule
// that came due while closed fires once when the calendar next opens (no
// catch-up burst). The gate applies only to starting an action or flow: a flow
// that started inside a window runs all of its steps even if a step crosses the
// window end, and agent work already in progress is never interrupted.
type CalendarConfig struct {
	// Timezone is an IANA zone name such as Asia/Taipei. Required; "Local" is
	// rejected so behavior never depends on the host timezone.
	Timezone string `yaml:"timezone"`
	// Weekdays lists the allowed days (mon..sun or monday..sunday). Required.
	Weekdays []string `yaml:"weekdays"`
	// Windows lists daily HH:MM ranges. Start is inclusive and end exclusive;
	// end may be 24:00. Overnight ranges are not supported — split them.
	Windows []CalendarWindow `yaml:"windows"`
}

type CalendarWindow struct {
	Start string `yaml:"start"`
	End   string `yaml:"end"`
}

// UnmarshalYAML decodes strictly so a misspelled calendar key fails validation
// instead of silently widening the schedule.
func (c *CalendarConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return errors.New("calendar: must be a mapping with timezone, weekdays, and windows")
	}
	type plain CalendarConfig
	var decoded plain
	if err := decodeStrict(value, &decoded, "calendar", "timezone", "weekdays", "windows"); err != nil {
		return err
	}
	*c = CalendarConfig(decoded)
	return nil
}

func (w *CalendarWindow) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return errors.New("calendar: each window must be a mapping with start and end")
	}
	type plain CalendarWindow
	var decoded plain
	if err := decodeStrict(value, &decoded, "calendar window", "start", "end"); err != nil {
		return err
	}
	*w = CalendarWindow(decoded)
	return nil
}

func decodeStrict(value *yaml.Node, out interface{}, context string, known ...string) error {
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		ok := false
		for _, name := range known {
			if key == name {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%s: unknown key %q (want %s)", context, key, strings.Join(known, ", "))
		}
	}
	return value.Decode(out)
}

// Calendar is the compiled, validated form of CalendarConfig.
type Calendar struct {
	location *time.Location
	weekdays [7]bool
	// windows are [start, end) offsets from local midnight in seconds, sorted
	// by start.
	windows []calendarRange
	config  CalendarConfig
}

type calendarRange struct {
	start int
	end   int
}

var calendarClockRE = regexp.MustCompile(`^([01][0-9]|2[0-4]):([0-5][0-9])$`)

var calendarWeekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

// CompileCalendar validates cfg and returns its compiled form. A nil cfg means
// the loop has no calendar restriction and returns (nil, nil).
func CompileCalendar(cfg *CalendarConfig) (*Calendar, error) {
	if cfg == nil {
		return nil, nil
	}
	name := strings.TrimSpace(cfg.Timezone)
	if name == "" {
		return nil, errors.New("calendar: timezone is required (IANA name such as Asia/Taipei)")
	}
	if name == "Local" {
		return nil, errors.New("calendar: timezone must be an explicit IANA name, not Local")
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("calendar: invalid timezone %q: %w", cfg.Timezone, err)
	}
	cal := &Calendar{location: location, config: *cfg}

	if len(cfg.Weekdays) == 0 {
		return nil, errors.New("calendar: weekdays is required (for example [mon, tue, wed, thu, fri])")
	}
	for _, raw := range cfg.Weekdays {
		day, ok := calendarWeekdayNames[strings.ToLower(strings.TrimSpace(raw))]
		if !ok {
			return nil, fmt.Errorf("calendar: invalid weekday %q (want mon..sun)", raw)
		}
		if cal.weekdays[day] {
			return nil, fmt.Errorf("calendar: duplicate weekday %q", raw)
		}
		cal.weekdays[day] = true
	}

	if len(cfg.Windows) == 0 {
		return nil, errors.New("calendar: at least one window is required")
	}
	for i, window := range cfg.Windows {
		start, err := parseCalendarClock(window.Start, false)
		if err != nil {
			return nil, fmt.Errorf("calendar: window %d start: %w", i+1, err)
		}
		end, err := parseCalendarClock(window.End, true)
		if err != nil {
			return nil, fmt.Errorf("calendar: window %d end: %w", i+1, err)
		}
		if end <= start {
			return nil, fmt.Errorf("calendar: window %d %s-%s must end after it starts (overnight ranges are not supported; split them)", i+1, window.Start, window.End)
		}
		cal.windows = append(cal.windows, calendarRange{start: start, end: end})
	}
	sort.Slice(cal.windows, func(i, j int) bool { return cal.windows[i].start < cal.windows[j].start })
	for i := 1; i < len(cal.windows); i++ {
		if cal.windows[i].start < cal.windows[i-1].end {
			return nil, errors.New("calendar: windows must not overlap")
		}
	}
	return cal, nil
}

func parseCalendarClock(value string, allowMidnightEnd bool) (int, error) {
	match := calendarClockRE.FindStringSubmatch(value)
	if match == nil {
		return 0, fmt.Errorf("invalid time %q (want HH:MM, 24-hour)", value)
	}
	hours, _ := strconv.Atoi(match[1])
	minutes, _ := strconv.Atoi(match[2])
	if hours == 24 {
		if !allowMidnightEnd || minutes != 0 {
			return 0, fmt.Errorf("invalid time %q (24:00 is only valid as a window end)", value)
		}
	}
	return hours*3600 + minutes*60, nil
}

// Location returns the calendar's timezone.
func (c *Calendar) Location() *time.Location { return c.location }

// Open reports whether an action or flow may start at t.
func (c *Calendar) Open(t time.Time) bool {
	_, ok := c.openWindow(t)
	return ok
}

// OpenUntil returns the end of the window containing t, if t is inside one.
func (c *Calendar) OpenUntil(t time.Time) (time.Time, bool) {
	window, ok := c.openWindow(t)
	if !ok {
		return time.Time{}, false
	}
	local := t.In(c.location)
	return c.at(local.Year(), local.Month(), local.Day(), window.end), true
}

func (c *Calendar) openWindow(t time.Time) (calendarRange, bool) {
	local := t.In(c.location)
	if !c.weekdays[local.Weekday()] {
		return calendarRange{}, false
	}
	second := local.Hour()*3600 + local.Minute()*60 + local.Second()
	for _, window := range c.windows {
		if second >= window.start && second < window.end {
			return window, true
		}
	}
	return calendarRange{}, false
}

// NextOpen returns the earliest instant at or after t at which the calendar is
// open. It returns t itself when the calendar is already open.
func (c *Calendar) NextOpen(t time.Time) time.Time {
	if c.Open(t) {
		return t
	}
	local := t.In(c.location)
	for offset := 0; offset <= 8; offset++ {
		day := time.Date(local.Year(), local.Month(), local.Day()+offset, 12, 0, 0, 0, c.location)
		if !c.weekdays[day.Weekday()] {
			continue
		}
		for _, window := range c.windows {
			open := c.at(day.Year(), day.Month(), day.Day(), window.start)
			if open.After(t) && c.Open(open) {
				return open
			}
		}
	}
	// Unreachable for a validated calendar (at least one weekday and window).
	return t
}

func (c *Calendar) at(year int, month time.Month, day, second int) time.Time {
	return time.Date(year, month, day, 0, 0, second, 0, c.location)
}

// Describe summarizes the calendar for previews, validation, and logs.
func (c *Calendar) Describe() map[string]interface{} {
	windows := make([]string, 0, len(c.config.Windows))
	for _, window := range c.config.Windows {
		windows = append(windows, window.Start+"-"+window.End)
	}
	days := make([]string, 0, 7)
	for _, day := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
		if c.weekdays[day] {
			days = append(days, strings.ToLower(day.String()[:3]))
		}
	}
	return map[string]interface{}{
		"timezone": c.location.String(),
		"weekdays": days,
		"windows":  windows,
	}
}
