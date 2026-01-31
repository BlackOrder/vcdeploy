package services

import (
	"testing"
	"time"
)

func TestNewPagination(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{"default values", 0, 0, 50, 0},
		{"negative limit", -1, 0, 50, 0},
		{"negative offset", 10, -5, 10, 0},
		{"exceeds max limit", 2000, 0, 1000, 0},
		{"valid values", 25, 100, 25, 100},
		{"exact max limit", 1000, 50, 1000, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := NewPagination(tt.limit, tt.offset)
			if pg.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", pg.Limit, tt.wantLimit)
			}
			if pg.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", pg.Offset, tt.wantOffset)
			}
		})
	}
}

func TestPagination_HasMore(t *testing.T) {
	tests := []struct {
		name       string
		pagination Pagination
		totalCount int64
		want       bool
	}{
		{"first page of many", Pagination{Limit: 10, Offset: 0}, 100, true},
		{"last page", Pagination{Limit: 10, Offset: 90}, 100, false},
		{"exact fit", Pagination{Limit: 10, Offset: 0}, 10, false},
		{"partial last page", Pagination{Limit: 10, Offset: 95}, 100, false},
		{"empty result", Pagination{Limit: 10, Offset: 0}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pagination.HasMore(tt.totalCount); got != tt.want {
				t.Errorf("HasMore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPagination_NextPage(t *testing.T) {
	pg := Pagination{Limit: 10, Offset: 0}
	next := pg.NextPage()

	if next.Limit != 10 {
		t.Errorf("NextPage Limit = %d, want 10", next.Limit)
	}
	if next.Offset != 10 {
		t.Errorf("NextPage Offset = %d, want 10", next.Offset)
	}

	// Chain multiple next pages
	next2 := next.NextPage()
	if next2.Offset != 20 {
		t.Errorf("NextPage().NextPage() Offset = %d, want 20", next2.Offset)
	}
}

func TestDateRange_IsValid(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		dateRange DateRange
		want      bool
	}{
		{"empty range", DateRange{}, false},
		{"only from", DateRange{From: now}, true},
		{"only to", DateRange{To: now}, true},
		{"both set", DateRange{From: now.Add(-time.Hour), To: now}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dateRange.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDateRange_Contains(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-time.Hour)
	hourLater := now.Add(time.Hour)

	tests := []struct {
		name      string
		dateRange DateRange
		checkTime time.Time
		want      bool
	}{
		{"empty range contains any", DateRange{}, now, true},
		{"within range", DateRange{From: hourAgo, To: hourLater}, now, true},
		{"before from", DateRange{From: now, To: hourLater}, hourAgo, false},
		{"after to", DateRange{From: hourAgo, To: now}, hourLater, false},
		{"only from - within", DateRange{From: hourAgo}, now, true},
		{"only from - before", DateRange{From: now}, hourAgo, false},
		{"only to - within", DateRange{To: hourLater}, now, true},
		{"only to - after", DateRange{To: now}, hourLater, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dateRange.Contains(tt.checkTime); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListResult_HasMore(t *testing.T) {
	result := ListResult[int]{
		Items:      []int{1, 2, 3},
		TotalCount: 100,
		Pagination: Pagination{Limit: 10, Offset: 0},
	}

	if !result.HasMore() {
		t.Error("Expected HasMore() to return true")
	}

	result.Pagination.Offset = 90
	if result.HasMore() {
		t.Error("Expected HasMore() to return false on last page")
	}
}

func TestListResult_IsEmpty(t *testing.T) {
	empty := ListResult[int]{
		Items:      []int{},
		TotalCount: 0,
		Pagination: Pagination{Limit: 10, Offset: 0},
	}

	if !empty.IsEmpty() {
		t.Error("Expected IsEmpty() to return true")
	}

	nonEmpty := ListResult[int]{
		Items:      []int{1},
		TotalCount: 1,
		Pagination: Pagination{Limit: 10, Offset: 0},
	}

	if nonEmpty.IsEmpty() {
		t.Error("Expected IsEmpty() to return false")
	}
}

func TestNewSortOption(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		direction     SortDirection
		wantDirection SortDirection
	}{
		{"ascending", "name", SortAsc, SortAsc},
		{"descending", "created_at", SortDesc, SortDesc},
		{"invalid direction defaults to asc", "name", "invalid", SortAsc},
		{"empty direction defaults to asc", "name", "", SortAsc},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := NewSortOption(tt.field, tt.direction)
			if opt.Field != tt.field {
				t.Errorf("Field = %q, want %q", opt.Field, tt.field)
			}
			if opt.Direction != tt.wantDirection {
				t.Errorf("Direction = %q, want %q", opt.Direction, tt.wantDirection)
			}
		})
	}
}

func TestAuditFilter(t *testing.T) {
	filter := AuditFilter{
		Action: "create",
		DateRange: DateRange{
			From: time.Now().Add(-24 * time.Hour),
			To:   time.Now(),
		},
		Pagination: NewPagination(20, 0),
	}

	if filter.Action != "create" {
		t.Errorf("Action = %q, want %q", filter.Action, "create")
	}
	if filter.Pagination.Limit != 20 {
		t.Errorf("Pagination.Limit = %d, want 20", filter.Pagination.Limit)
	}
	if !filter.DateRange.IsValid() {
		t.Error("Expected DateRange to be valid")
	}
}

func TestDeploymentFilter(t *testing.T) {
	filter := DeploymentFilter{
		Project: "myapp",
		DateRange: DateRange{
			From: time.Now().Add(-7 * 24 * time.Hour),
		},
		Pagination: NewPagination(50, 100),
	}

	if filter.Project != "myapp" {
		t.Errorf("Project = %q, want %q", filter.Project, "myapp")
	}
	if filter.Pagination.Offset != 100 {
		t.Errorf("Pagination.Offset = %d, want 100", filter.Pagination.Offset)
	}
}
