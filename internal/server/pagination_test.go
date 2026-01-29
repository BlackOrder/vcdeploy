package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParsePagination_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := parsePagination(req)

	if p.Limit != DefaultPageSize {
		t.Errorf("Limit = %d, want %d", p.Limit, DefaultPageSize)
	}
	if p.Offset != 0 {
		t.Errorf("Offset = %d, want 0", p.Offset)
	}
}

func TestParsePagination_WithLimit(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantLimit int
	}{
		{"valid limit", "?limit=25", 25},
		{"limit at max", "?limit=1000", 1000},
		{"limit exceeds max", "?limit=2000", MaxPageSize},
		{"limit zero uses default", "?limit=0", DefaultPageSize},
		{"limit negative uses default", "?limit=-5", DefaultPageSize},
		{"invalid limit uses default", "?limit=abc", DefaultPageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test"+tt.query, nil)
			p := parsePagination(req)
			if p.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", p.Limit, tt.wantLimit)
			}
		})
	}
}

func TestParsePagination_WithOffset(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantOffset int
	}{
		{"valid offset", "?offset=100", 100},
		{"offset zero", "?offset=0", 0},
		{"negative offset uses zero", "?offset=-10", 0},
		{"invalid offset uses zero", "?offset=xyz", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test"+tt.query, nil)
			p := parsePagination(req)
			if p.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", p.Offset, tt.wantOffset)
			}
		})
	}
}

func TestParsePagination_WithPage(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantOffset int
	}{
		{"page 1", "?page=1", 0},
		{"page 2", "?page=2", 50},
		{"page 3 with limit 20", "?page=3&limit=20", 40},
		{"page zero ignored", "?page=0", 0},
		{"negative page ignored", "?page=-1", 0},
		{"invalid page ignored", "?page=abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test"+tt.query, nil)
			p := parsePagination(req)
			if p.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", p.Offset, tt.wantOffset)
			}
		})
	}
}

func TestParsePagination_OffsetOverridesPage(t *testing.T) {
	// When both offset and page are provided, offset takes precedence
	req := httptest.NewRequest(http.MethodGet, "/test?offset=75&page=10", nil)
	p := parsePagination(req)

	if p.Offset != 75 {
		t.Errorf("Offset = %d, want 75 (offset should override page)", p.Offset)
	}
}

func TestParsePaginationWithDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := parsePaginationWithDefaults(req, 25)

	if p.Limit != 25 {
		t.Errorf("Limit = %d, want 25", p.Limit)
	}
}

func TestParsePaginationWithDefaults_OverridesCustomDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?limit=100", nil)
	p := parsePaginationWithDefaults(req, 25)

	if p.Limit != 100 {
		t.Errorf("Limit = %d, want 100 (query param should override default)", p.Limit)
	}
}

func TestNewPaginationInfo_Basic(t *testing.T) {
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10", nil))
	info := NewPaginationInfo(p, 100)

	if info.Limit != 10 {
		t.Errorf("Limit = %d, want 10", info.Limit)
	}
	if info.Offset != 0 {
		t.Errorf("Offset = %d, want 0", info.Offset)
	}
	if info.TotalCount != 100 {
		t.Errorf("TotalCount = %d, want 100", info.TotalCount)
	}
	if info.CurrentPage != 1 {
		t.Errorf("CurrentPage = %d, want 1", info.CurrentPage)
	}
	if info.TotalPages != 10 {
		t.Errorf("TotalPages = %d, want 10", info.TotalPages)
	}
	if info.HasPrev {
		t.Error("HasPrev should be false on first page")
	}
	if !info.HasNext {
		t.Error("HasNext should be true when more pages exist")
	}
}

func TestNewPaginationInfo_MiddlePage(t *testing.T) {
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10&offset=50", nil))
	info := NewPaginationInfo(p, 100)

	if info.CurrentPage != 6 {
		t.Errorf("CurrentPage = %d, want 6", info.CurrentPage)
	}
	if !info.HasPrev {
		t.Error("HasPrev should be true on middle page")
	}
	if !info.HasNext {
		t.Error("HasNext should be true on middle page")
	}
	if info.PrevOffset != 40 {
		t.Errorf("PrevOffset = %d, want 40", info.PrevOffset)
	}
	if info.NextOffset != 60 {
		t.Errorf("NextOffset = %d, want 60", info.NextOffset)
	}
}

func TestNewPaginationInfo_LastPage(t *testing.T) {
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10&offset=90", nil))
	info := NewPaginationInfo(p, 100)

	if info.CurrentPage != 10 {
		t.Errorf("CurrentPage = %d, want 10", info.CurrentPage)
	}
	if !info.HasPrev {
		t.Error("HasPrev should be true on last page")
	}
	if info.HasNext {
		t.Error("HasNext should be false on last page")
	}
}

func TestNewPaginationInfo_ZeroTotal(t *testing.T) {
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10", nil))
	info := NewPaginationInfo(p, 0)

	if info.TotalPages != 1 {
		t.Errorf("TotalPages = %d, want 1 (minimum)", info.TotalPages)
	}
	if info.HasNext {
		t.Error("HasNext should be false with zero items")
	}
}

func TestNewPaginationInfo_PartialPage(t *testing.T) {
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10", nil))
	info := NewPaginationInfo(p, 5)

	if info.TotalPages != 1 {
		t.Errorf("TotalPages = %d, want 1", info.TotalPages)
	}
	if info.HasNext {
		t.Error("HasNext should be false when all items fit on one page")
	}
}

func TestNewPaginationInfo_PrevOffsetClampedToZero(t *testing.T) {
	// First page should have PrevOffset of 0, not negative
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10&offset=0", nil))
	info := NewPaginationInfo(p, 100)

	if info.PrevOffset != 0 {
		t.Errorf("PrevOffset = %d, want 0 (should not go negative)", info.PrevOffset)
	}
}

func TestGeneratePageNumbers_LessThanMax(t *testing.T) {
	pages := generatePageNumbers(1, 3, 5)
	expected := []int{1, 2, 3}

	if len(pages) != len(expected) {
		t.Fatalf("got %d pages, want %d", len(pages), len(expected))
	}
	for i, p := range pages {
		if p != expected[i] {
			t.Errorf("page[%d] = %d, want %d", i, p, expected[i])
		}
	}
}

func TestGeneratePageNumbers_ExactlyMax(t *testing.T) {
	pages := generatePageNumbers(3, 5, 5)
	expected := []int{1, 2, 3, 4, 5}

	if len(pages) != len(expected) {
		t.Fatalf("got %d pages, want %d", len(pages), len(expected))
	}
	for i, p := range pages {
		if p != expected[i] {
			t.Errorf("page[%d] = %d, want %d", i, p, expected[i])
		}
	}
}

func TestGeneratePageNumbers_CenteredAroundCurrent(t *testing.T) {
	pages := generatePageNumbers(5, 10, 5)
	expected := []int{3, 4, 5, 6, 7}

	if len(pages) != len(expected) {
		t.Fatalf("got %d pages, want %d", len(pages), len(expected))
	}
	for i, p := range pages {
		if p != expected[i] {
			t.Errorf("page[%d] = %d, want %d", i, p, expected[i])
		}
	}
}

func TestGeneratePageNumbers_NearStart(t *testing.T) {
	// When current page is near start, should show pages 1-5
	pages := generatePageNumbers(2, 10, 5)
	expected := []int{1, 2, 3, 4, 5}

	if len(pages) != len(expected) {
		t.Fatalf("got %d pages, want %d", len(pages), len(expected))
	}
	for i, p := range pages {
		if p != expected[i] {
			t.Errorf("page[%d] = %d, want %d", i, p, expected[i])
		}
	}
}

func TestGeneratePageNumbers_NearEnd(t *testing.T) {
	// When current page is near end, should show last 5 pages
	pages := generatePageNumbers(9, 10, 5)
	expected := []int{6, 7, 8, 9, 10}

	if len(pages) != len(expected) {
		t.Fatalf("got %d pages, want %d", len(pages), len(expected))
	}
	for i, p := range pages {
		if p != expected[i] {
			t.Errorf("page[%d] = %d, want %d", i, p, expected[i])
		}
	}
}

func TestGeneratePageNumbers_AtEnd(t *testing.T) {
	// When on last page
	pages := generatePageNumbers(10, 10, 5)
	expected := []int{6, 7, 8, 9, 10}

	if len(pages) != len(expected) {
		t.Fatalf("got %d pages, want %d", len(pages), len(expected))
	}
	for i, p := range pages {
		if p != expected[i] {
			t.Errorf("page[%d] = %d, want %d", i, p, expected[i])
		}
	}
}

func TestGeneratePageNumbers_SinglePage(t *testing.T) {
	pages := generatePageNumbers(1, 1, 5)
	expected := []int{1}

	if len(pages) != len(expected) {
		t.Fatalf("got %d pages, want %d", len(pages), len(expected))
	}
	if pages[0] != 1 {
		t.Errorf("page[0] = %d, want 1", pages[0])
	}
}

func TestNewPaginationInfo_PageNumbers(t *testing.T) {
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10&offset=40", nil))
	info := NewPaginationInfo(p, 100)

	// Page 5 of 10, should show pages 3,4,5,6,7
	if len(info.PageNumbers) != 5 {
		t.Errorf("got %d page numbers, want 5", len(info.PageNumbers))
	}

	expected := []int{3, 4, 5, 6, 7}
	for i, num := range info.PageNumbers {
		if num != expected[i] {
			t.Errorf("PageNumbers[%d] = %d, want %d", i, num, expected[i])
		}
	}
}

func BenchmarkParsePagination(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/test?limit=25&page=5", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parsePagination(req)
	}
}

func BenchmarkNewPaginationInfo(b *testing.B) {
	p := parsePagination(httptest.NewRequest(http.MethodGet, "/test?limit=10&offset=50", nil))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewPaginationInfo(p, 1000)
	}
}

func BenchmarkGeneratePageNumbers(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generatePageNumbers(50, 100, 5)
	}
}
