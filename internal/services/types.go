package services

import "time"

// Pagination contains pagination parameters.
type Pagination struct {
	Limit  int
	Offset int
}

// NewPagination creates pagination with defaults and bounds checking.
func NewPagination(limit, offset int) Pagination {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return Pagination{Limit: limit, Offset: offset}
}

// HasMore returns true if there are likely more results beyond the current page.
func (p Pagination) HasMore(totalCount int64) bool {
	return int64(p.Offset+p.Limit) < totalCount
}

// NextPage returns pagination for the next page.
func (p Pagination) NextPage() Pagination {
	return Pagination{
		Limit:  p.Limit,
		Offset: p.Offset + p.Limit,
	}
}

// DateRange represents a time range for filtering.
type DateRange struct {
	From time.Time
	To   time.Time
}

// IsValid returns true if the date range has at least one bound set.
func (d DateRange) IsValid() bool {
	return !d.From.IsZero() || !d.To.IsZero()
}

// Contains returns true if the given time is within the date range.
func (d DateRange) Contains(t time.Time) bool {
	if !d.From.IsZero() && t.Before(d.From) {
		return false
	}
	if !d.To.IsZero() && t.After(d.To) {
		return false
	}
	return true
}

// AuditFilter contains criteria for filtering audit logs.
type AuditFilter struct {
	Action   string
	Resource string
	User     string
	DateRange
	Pagination
}

// DeploymentFilter contains criteria for filtering deployments.
type DeploymentFilter struct {
	Project string
	Status  string
	Agent   string
	DateRange
	Pagination
}

// ListResult wraps list results with total count for pagination.
type ListResult[T any] struct {
	Items      []T
	TotalCount int64
	Pagination Pagination
}

// HasMore returns true if there are more pages available.
func (r ListResult[T]) HasMore() bool {
	return r.Pagination.HasMore(r.TotalCount)
}

// IsEmpty returns true if there are no items.
func (r ListResult[T]) IsEmpty() bool {
	return len(r.Items) == 0
}

// SortDirection represents the direction of sorting.
type SortDirection string

const (
	// SortAsc sorts in ascending order.
	SortAsc SortDirection = "asc"
	// SortDesc sorts in descending order.
	SortDesc SortDirection = "desc"
)

// SortOption represents a sorting option.
type SortOption struct {
	Field     string
	Direction SortDirection
}

// NewSortOption creates a new sort option with validation.
func NewSortOption(field string, direction SortDirection) SortOption {
	if direction != SortAsc && direction != SortDesc {
		direction = SortAsc
	}
	return SortOption{Field: field, Direction: direction}
}
