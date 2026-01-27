package scheduler

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestParseCron(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name:    "every minute",
			expr:    "* * * * *",
			wantErr: false,
		},
		{
			name:    "daily at 2am",
			expr:    "0 2 * * *",
			wantErr: false,
		},
		{
			name:    "every hour on the hour",
			expr:    "0 * * * *",
			wantErr: false,
		},
		{
			name:    "every 15 minutes",
			expr:    "*/15 * * * *",
			wantErr: false,
		},
		{
			name:    "weekdays at 9am",
			expr:    "0 9 * * 1-5",
			wantErr: false,
		},
		{
			name:    "first of month at midnight",
			expr:    "0 0 1 * *",
			wantErr: false,
		},
		{
			name:    "comma separated hours",
			expr:    "0 2,4,6 * * *",
			wantErr: false,
		},
		{
			name:    "invalid - only 4 fields",
			expr:    "* * * *",
			wantErr: true,
		},
		{
			name:    "invalid - bad hour",
			expr:    "0 25 * * *",
			wantErr: true,
		},
		{
			name:    "invalid - bad minute",
			expr:    "60 * * * *",
			wantErr: true,
		},
		{
			name:    "invalid - bad step",
			expr:    "*/0 * * * *",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCron(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCron(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name:    "duration - 1 hour",
			expr:    "1h",
			wantErr: false,
		},
		{
			name:    "duration - 30 minutes",
			expr:    "30m",
			wantErr: false,
		},
		{
			name:    "duration - 24 hours",
			expr:    "24h",
			wantErr: false,
		},
		{
			name:    "keyword - daily",
			expr:    "@daily",
			wantErr: false,
		},
		{
			name:    "keyword - hourly",
			expr:    "@hourly",
			wantErr: false,
		},
		{
			name:    "keyword - weekly",
			expr:    "@weekly",
			wantErr: false,
		},
		{
			name:    "keyword - monthly",
			expr:    "@monthly",
			wantErr: false,
		},
		{
			name:    "cron expression",
			expr:    "0 2 * * *",
			wantErr: false,
		},
		{
			name:    "invalid duration too short",
			expr:    "30s",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSchedule(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSchedule(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestIntervalSchedule_Next(t *testing.T) {
	schedule := &IntervalSchedule{Interval: time.Hour}
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	next := schedule.Next(now)
	expected := now.Add(time.Hour)

	if !next.Equal(expected) {
		t.Errorf("IntervalSchedule.Next() = %v, want %v", next, expected)
	}
}

func TestDailySchedule_Next(t *testing.T) {
	tests := []struct {
		name     string
		hour     int
		minute   int
		after    time.Time
		expected time.Time
	}{
		{
			name:     "before scheduled time",
			hour:     10,
			minute:   0,
			after:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "after scheduled time",
			hour:     10,
			minute:   0,
			after:    time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "at scheduled time",
			hour:     10,
			minute:   0,
			after:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &DailySchedule{Hour: tt.hour, Minute: tt.minute}
			next := schedule.Next(tt.after)
			if !next.Equal(tt.expected) {
				t.Errorf("DailySchedule.Next() = %v, want %v", next, tt.expected)
			}
		})
	}
}

func TestCronSchedule_Next(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		after    time.Time
		expected time.Time
	}{
		{
			name:     "every hour on the hour",
			expr:     "0 * * * *",
			after:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		},
		{
			name:     "daily at 2am",
			expr:     "0 2 * * *",
			after:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 16, 2, 0, 0, 0, time.UTC),
		},
		{
			name:     "daily at 2am (before)",
			expr:     "0 2 * * *",
			after:    time.Date(2024, 1, 15, 1, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 15, 2, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := ParseCron(tt.expr)
			if err != nil {
				t.Fatalf("ParseCron(%q) error = %v", tt.expr, err)
			}

			next := schedule.Next(tt.after)
			if !next.Equal(tt.expected) {
				t.Errorf("CronSchedule.Next() = %v, want %v", next, tt.expected)
			}
		})
	}
}

func TestScheduler_AddRemoveJob(t *testing.T) {
	logger := zap.NewNop()
	sched := New(logger)

	job := &testJob{name: "test-job", enabled: true}
	schedule := &IntervalSchedule{Interval: time.Hour}

	// Add job
	err := sched.AddJob(job, schedule)
	if err != nil {
		t.Fatalf("AddJob() error = %v", err)
	}

	// Verify job exists
	status := sched.GetJobStatus()
	if _, ok := status["test-job"]; !ok {
		t.Error("Job not found after adding")
	}

	// Try adding duplicate
	err = sched.AddJob(job, schedule)
	if err == nil {
		t.Error("Expected error when adding duplicate job")
	}

	// Remove job
	sched.RemoveJob("test-job")
	status = sched.GetJobStatus()
	if _, ok := status["test-job"]; ok {
		t.Error("Job found after removing")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	logger := zap.NewNop()
	sched := New(logger)

	// Start scheduler
	sched.Start()

	// Start again (should be idempotent)
	sched.Start()

	// Stop scheduler
	sched.Stop()

	// Stop again (should be idempotent)
	sched.Stop()
}

func TestScheduler_RunJobNow(t *testing.T) {
	logger := zap.NewNop()
	sched := New(logger)

	job := &testJob{name: "test-job", enabled: true}
	schedule := &IntervalSchedule{Interval: time.Hour}

	err := sched.AddJob(job, schedule)
	if err != nil {
		t.Fatalf("AddJob() error = %v", err)
	}

	// Run job now
	err = sched.RunJobNow("test-job")
	if err != nil {
		t.Errorf("RunJobNow() error = %v", err)
	}

	if !job.ran {
		t.Error("Job was not executed")
	}

	// Try running non-existent job
	err = sched.RunJobNow("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent job")
	}
}

func TestScheduler_JobStatus(t *testing.T) {
	logger := zap.NewNop()
	sched := New(logger)

	job1 := &testJob{name: "job1", enabled: true}
	job2 := &testJob{name: "job2", enabled: false}

	sched.AddJob(job1, &IntervalSchedule{Interval: time.Hour})
	sched.AddJob(job2, &DailySchedule{Hour: 2, Minute: 0})

	status := sched.GetJobStatus()

	if len(status) != 2 {
		t.Errorf("Expected 2 jobs, got %d", len(status))
	}

	if !status["job1"].Enabled {
		t.Error("job1 should be enabled")
	}

	if status["job2"].Enabled {
		t.Error("job2 should be disabled")
	}
}

// testJob is a simple job implementation for testing
type testJob struct {
	name    string
	enabled bool
	ran     bool
	err     error
}

func (j *testJob) Name() string {
	return j.name
}

func (j *testJob) Enabled() bool {
	return j.enabled
}

func (j *testJob) Run(_ context.Context) error {
	j.ran = true
	return j.err
}

func TestParseField(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		min     int
		max     int
		want    []int
		wantErr bool
	}{
		{
			name:  "wildcard",
			field: "*",
			min:   0,
			max:   2,
			want:  []int{0, 1, 2},
		},
		{
			name:  "step",
			field: "*/2",
			min:   0,
			max:   6,
			want:  []int{0, 2, 4, 6},
		},
		{
			name:  "single value",
			field: "5",
			min:   0,
			max:   10,
			want:  []int{5},
		},
		{
			name:  "range",
			field: "1-3",
			min:   0,
			max:   5,
			want:  []int{1, 2, 3},
		},
		{
			name:  "comma separated",
			field: "1,3,5",
			min:   0,
			max:   10,
			want:  []int{1, 3, 5},
		},
		{
			name:    "value out of range",
			field:   "100",
			min:     0,
			max:     59,
			wantErr: true,
		},
		{
			name:    "invalid step",
			field:   "*/0",
			min:     0,
			max:     59,
			wantErr: true,
		},
		{
			name:    "invalid range",
			field:   "5-3",
			min:     0,
			max:     10,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseField(tt.field, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseField() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !equalSlice(got, tt.want) {
				t.Errorf("parseField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func equalSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
