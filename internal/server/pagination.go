// Package server provides API endpoint handlers.
package server

import (
	"net/http"
	"strconv"

	"github.com/BlackOrder/vcdeploy/internal/services"
)

const (
	// DefaultPageSize is the default number of items per page.
	DefaultPageSize = 50
	// MaxPageSize is the maximum allowed page size.
	MaxPageSize = 1000
)

// parsePagination extracts pagination parameters from query string.
// Query parameters: limit (default 50), offset (default 0)
// Also supports page parameter: page=2 with limit=50 means offset=50
func parsePagination(r *http.Request) services.Pagination {
	return parsePaginationWithDefaults(r, DefaultPageSize)
}

// parsePaginationWithDefaults extracts pagination with a custom default limit.
// Query parameters: limit, offset, page
// Limit is capped at MaxPageSize.
func parsePaginationWithDefaults(r *http.Request, defaultLimit int) services.Pagination {
	q := r.URL.Query()

	// Parse limit
	limit := defaultLimit
	if limitStr := q.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	// Cap at max
	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	// Parse offset, or calculate from page
	offset := 0
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	} else if pageStr := q.Get("page"); pageStr != "" {
		// Page is 1-indexed
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			offset = (p - 1) * limit
		}
	}

	return services.NewPagination(limit, offset)
}

// PaginationInfo contains pagination metadata for templates.
type PaginationInfo struct {
	Limit       int
	Offset      int
	TotalCount  int64
	CurrentPage int
	TotalPages  int
	HasPrev     bool
	HasNext     bool
	PrevOffset  int
	NextOffset  int
	PageNumbers []int // Limited range of page numbers for display
}

// NewPaginationInfo creates pagination info from services types.
func NewPaginationInfo(p services.Pagination, totalCount int64) PaginationInfo {
	currentPage := (p.Offset / p.Limit) + 1
	totalPages := int((totalCount + int64(p.Limit) - 1) / int64(p.Limit))
	if totalPages == 0 {
		totalPages = 1
	}

	info := PaginationInfo{
		Limit:       p.Limit,
		Offset:      p.Offset,
		TotalCount:  totalCount,
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		HasPrev:     p.Offset > 0,
		HasNext:     p.HasMore(totalCount),
		PrevOffset:  max(0, p.Offset-p.Limit),
		NextOffset:  p.Offset + p.Limit,
	}

	// Generate page numbers (show 5 pages around current)
	info.PageNumbers = generatePageNumbers(currentPage, totalPages, 5)

	return info
}

// generatePageNumbers returns a slice of page numbers to display.
// Shows up to maxVisible pages, centered around current page.
func generatePageNumbers(current, total, maxVisible int) []int {
	if total <= maxVisible {
		pages := make([]int, total)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages
	}

	// Calculate start and end
	half := maxVisible / 2
	start := current - half
	end := current + half

	if start < 1 {
		start = 1
		end = maxVisible
	}
	if end > total {
		end = total
		start = total - maxVisible + 1
	}

	pages := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		pages = append(pages, i)
	}
	return pages
}
