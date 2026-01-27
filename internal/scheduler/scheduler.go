// Package scheduler provides a flexible job scheduling system for background tasks.
package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Job represents a scheduled job that can be executed.
type Job interface {
	// Name returns the job's unique identifier.
	Name() string
	// Run executes the job.
	Run(ctx context.Context) error
	// Enabled returns whether the job should run.
	Enabled() bool
}

// Schedule represents when a job should run.
type Schedule interface {
	// Next returns the next time the job should run after the given time.
	Next(after time.Time) time.Time
}

// Scheduler manages scheduled jobs.
type Scheduler struct {
	jobs     map[string]*jobEntry
	mu       sync.RWMutex
	logger   *zap.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
	running  bool
	runMu    sync.Mutex
	location *time.Location
}

type jobEntry struct {
	job      Job
	schedule Schedule
	lastRun  time.Time
	nextRun  time.Time
}

// New creates a new Scheduler.
func New(logger *zap.Logger) *Scheduler {
	return &Scheduler{
		jobs:     make(map[string]*jobEntry),
		logger:   logger,
		stopCh:   make(chan struct{}),
		location: time.Local,
	}
}

// AddJob adds a job with the given schedule.
func (s *Scheduler) AddJob(job Job, schedule Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := job.Name()
	if _, exists := s.jobs[name]; exists {
		return fmt.Errorf("job %q already exists", name)
	}

	entry := &jobEntry{
		job:      job,
		schedule: schedule,
		nextRun:  schedule.Next(time.Now().In(s.location)),
	}
	s.jobs[name] = entry

	s.logger.Info("Job added",
		zap.String("job", name),
		zap.Time("nextRun", entry.nextRun),
		zap.Bool("enabled", job.Enabled()),
	)

	return nil
}

// RemoveJob removes a job by name.
func (s *Scheduler) RemoveJob(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, name)
	s.logger.Info("Job removed", zap.String("job", name))
}

// Start starts the scheduler.
func (s *Scheduler) Start() {
	s.runMu.Lock()
	if s.running {
		s.runMu.Unlock()
		return
	}
	s.running = true
	s.runMu.Unlock()

	s.wg.Go(s.run)
	s.logger.Info("Scheduler started")
}

// Stop stops the scheduler and waits for running jobs to complete.
func (s *Scheduler) Stop() {
	s.runMu.Lock()
	if !s.running {
		s.runMu.Unlock()
		return
	}
	s.running = false
	s.runMu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("Scheduler stopped")
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runDueJobs()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runDueJobs() {
	s.mu.Lock()
	now := time.Now().In(s.location)
	dueJobs := make([]*jobEntry, 0)

	for _, entry := range s.jobs {
		if !entry.job.Enabled() {
			continue
		}
		if now.After(entry.nextRun) || now.Equal(entry.nextRun) {
			dueJobs = append(dueJobs, entry)
		}
	}
	s.mu.Unlock()

	for _, entry := range dueJobs {
		s.executeJob(entry, now)
	}
}

func (s *Scheduler) executeJob(entry *jobEntry, now time.Time) {
	name := entry.job.Name()
	s.logger.Debug("Running job", zap.String("job", name))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	start := time.Now()
	err := entry.job.Run(ctx)
	duration := time.Since(start)

	s.mu.Lock()
	entry.lastRun = now
	entry.nextRun = entry.schedule.Next(now)
	s.mu.Unlock()

	if err != nil {
		s.logger.Error("Job failed",
			zap.String("job", name),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
	} else {
		s.logger.Info("Job completed",
			zap.String("job", name),
			zap.Duration("duration", duration),
			zap.Time("nextRun", entry.nextRun),
		)
	}
}

// RunJobNow runs a job immediately, outside of its schedule.
func (s *Scheduler) RunJobNow(name string) error {
	s.mu.RLock()
	entry, exists := s.jobs[name]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job %q not found", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	return entry.job.Run(ctx)
}

// GetJobStatus returns the status of all jobs.
func (s *Scheduler) GetJobStatus() map[string]JobStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make(map[string]JobStatus)
	for name, entry := range s.jobs {
		status[name] = JobStatus{
			Name:     name,
			Enabled:  entry.job.Enabled(),
			LastRun:  entry.lastRun,
			NextRun:  entry.nextRun,
		}
	}
	return status
}

// JobStatus represents the current state of a job.
type JobStatus struct {
	Name    string
	Enabled bool
	LastRun time.Time
	NextRun time.Time
}

// --- Schedule implementations ---

// IntervalSchedule runs a job at fixed intervals.
type IntervalSchedule struct {
	Interval time.Duration
}

// Next returns the next run time.
func (s *IntervalSchedule) Next(after time.Time) time.Time {
	return after.Add(s.Interval)
}

// DailySchedule runs a job daily at a specific time.
type DailySchedule struct {
	Hour   int
	Minute int
}

// Next returns the next run time.
func (s *DailySchedule) Next(after time.Time) time.Time {
	next := time.Date(after.Year(), after.Month(), after.Day(), s.Hour, s.Minute, 0, 0, after.Location())
	if !next.After(after) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// CronSchedule represents a cron-like schedule.
// Supports: "0 2 * * *" (minute hour day month weekday)
type CronSchedule struct {
	Minute  []int // 0-59
	Hour    []int // 0-23
	Day     []int // 1-31
	Month   []int // 1-12
	Weekday []int // 0-6 (Sunday = 0)
}

// ParseCron parses a cron expression.
// Format: "minute hour day month weekday"
// Supports: *, */N, N, N-M, N,M,O
func ParseCron(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	day, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day field: %w", err)
	}

	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	weekday, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid weekday field: %w", err)
	}

	return &CronSchedule{
		Minute:  minute,
		Hour:    hour,
		Day:     day,
		Month:   month,
		Weekday: weekday,
	}, nil
}

// parseField parses a single cron field.
func parseField(field string, min, max int) ([]int, error) {
	var values []int

	// Handle *
	if field == "*" {
		for i := min; i <= max; i++ {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle */N (step)
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step value: %s", field)
		}
		for i := min; i <= max; i += step {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle comma-separated values
	parts := strings.Split(field, ",")
	for _, part := range parts {
		// Handle range N-M
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
			}
			if start < min || end > max || start > end {
				return nil, fmt.Errorf("range out of bounds: %s", part)
			}
			for i := start; i <= end; i++ {
				values = append(values, i)
			}
		} else {
			// Single value
			val, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value: %s", part)
			}
			if val < min || val > max {
				return nil, fmt.Errorf("value out of bounds: %d (must be %d-%d)", val, min, max)
			}
			values = append(values, val)
		}
	}

	return values, nil
}

// Next returns the next run time for the cron schedule.
func (s *CronSchedule) Next(after time.Time) time.Time {
	// Start from the next minute
	t := after.Add(time.Minute).Truncate(time.Minute)

	// Search up to 4 years for next valid time
	maxIterations := 4 * 365 * 24 * 60
	for i := 0; i < maxIterations; i++ {
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}

	// Fallback: return far future
	return after.AddDate(10, 0, 0)
}

func (s *CronSchedule) matches(t time.Time) bool {
	return contains(s.Minute, t.Minute()) &&
		contains(s.Hour, t.Hour()) &&
		contains(s.Day, t.Day()) &&
		contains(s.Month, int(t.Month())) &&
		contains(s.Weekday, int(t.Weekday()))
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// ParseSchedule parses a schedule string and returns the appropriate Schedule.
// Supports:
// - Cron expressions: "0 2 * * *"
// - Duration strings: "24h", "1h30m"
// - Special keywords: "@daily", "@hourly", "@weekly", "@monthly"
func ParseSchedule(expr string) (Schedule, error) {
	expr = strings.TrimSpace(expr)

	// Handle special keywords
	switch strings.ToLower(expr) {
	case "@daily", "@midnight":
		return &DailySchedule{Hour: 0, Minute: 0}, nil
	case "@hourly":
		return &IntervalSchedule{Interval: time.Hour}, nil
	case "@weekly":
		return &CronSchedule{
			Minute:  []int{0},
			Hour:    []int{0},
			Day:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			Month:   []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			Weekday: []int{0}, // Sunday
		}, nil
	case "@monthly":
		return &CronSchedule{
			Minute:  []int{0},
			Hour:    []int{0},
			Day:     []int{1},
			Month:   []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			Weekday: []int{0, 1, 2, 3, 4, 5, 6},
		}, nil
	}

	// Try to parse as duration
	if d, err := time.ParseDuration(expr); err == nil {
		if d < time.Minute {
			return nil, fmt.Errorf("interval must be at least 1 minute, got %v", d)
		}
		return &IntervalSchedule{Interval: d}, nil
	}

	// Try to parse as cron expression
	return ParseCron(expr)
}
