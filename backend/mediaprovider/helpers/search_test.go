package helpers

import (
	"testing"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
)

func TestAllTermsMatch(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		terms    []string
		expected bool
	}{
		{
			name:     "all terms match",
			text:     "the quick brown fox",
			terms:    []string{"quick", "brown"},
			expected: true,
		},
		{
			name:     "one term missing",
			text:     "the quick brown fox",
			terms:    []string{"quick", "slow"},
			expected: false,
		},
		{
			name:     "empty terms",
			text:     "the quick brown fox",
			terms:    []string{},
			expected: true,
		},
		{
			name:     "single term matches",
			text:     "hello world",
			terms:    []string{"hello"},
			expected: true,
		},
		{
			name:     "single term does not match",
			text:     "hello world",
			terms:    []string{"goodbye"},
			expected: false,
		},
		{
			name:     "partial word match",
			text:     "testing",
			terms:    []string{"test"},
			expected: true,
		},
		{
			name:     "case sensitive - lowercase text and terms",
			text:     "the quick brown fox",
			terms:    []string{"the", "fox"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AllTermsMatch(tt.text, tt.terms)
			if result != tt.expected {
				t.Errorf("AllTermsMatch(%q, %v) = %v, want %v",
					tt.text, tt.terms, result, tt.expected)
			}
		})
	}
}

func TestRankSearchResults(t *testing.T) {
	tests := []struct {
		name       string
		results    []*mediaprovider.SearchResult
		fullQuery  string
		queryTerms []string
		expected   []string // expected order of result names
	}{
		{
			name:       "empty results",
			results:    []*mediaprovider.SearchResult{},
			fullQuery:  "test",
			queryTerms: []string{"test"},
			expected:   []string{},
		},
		{
			name: "single result",
			results: []*mediaprovider.SearchResult{
				{Name: "Test Song", Type: mediaprovider.ContentTypeTrack},
			},
			fullQuery:  "test",
			queryTerms: []string{"test"},
			expected:   []string{"Test Song"},
		},
		{
			name: "full query match vs partial",
			results: []*mediaprovider.SearchResult{
				{Name: "The Beatles", Type: mediaprovider.ContentTypeArtist},
				{Name: "Beatles Rock Band", Type: mediaprovider.ContentTypeArtist},
			},
			fullQuery:  "beatles",
			queryTerms: []string{"beatles"},
			expected:   []string{"Beatles Rock Band", "The Beatles"}, // "Beatles Rock Band" matches at position 0
		},
		{
			name: "earlier position wins",
			results: []*mediaprovider.SearchResult{
				{Name: "Rock The Beatles", Type: mediaprovider.ContentTypeAlbum},
				{Name: "The Beatles Rock", Type: mediaprovider.ContentTypeAlbum},
			},
			fullQuery:  "beatles",
			queryTerms: []string{"beatles"},
			expected:   []string{"The Beatles Rock", "Rock The Beatles"},
		},
		{
			name: "type priority - albums before tracks",
			results: []*mediaprovider.SearchResult{
				{Name: "Abbey Road", Type: mediaprovider.ContentTypeTrack},
				{Name: "Abbey Road", Type: mediaprovider.ContentTypeAlbum},
			},
			fullQuery:  "abbey",
			queryTerms: []string{"abbey"},
			expected:   []string{"Abbey Road", "Abbey Road"}, // Album (Type=0) comes before Track (Type=3)
		},
		{
			name: "empty query terms",
			results: []*mediaprovider.SearchResult{
				{Name: "Song A", Type: mediaprovider.ContentTypeTrack},
				{Name: "Song B", Type: mediaprovider.ContentTypeTrack},
			},
			fullQuery:  "",
			queryTerms: []string{},
			expected:   []string{"Song A", "Song B"}, // no reordering
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying original
			results := make([]*mediaprovider.SearchResult, len(tt.results))
			copy(results, tt.results)

			RankSearchResults(results, tt.fullQuery, tt.queryTerms)

			if len(results) != len(tt.expected) {
				t.Fatalf("Expected %d results, got %d", len(tt.expected), len(results))
			}

			for i, expectedName := range tt.expected {
				if results[i].Name != expectedName {
					t.Errorf("Position %d: expected %q, got %q",
						i, expectedName, results[i].Name)
				}
			}
		})
	}
}

func TestRankSearchResults_ComplexScenario(t *testing.T) {
	// Test a more realistic scenario with multiple factors
	results := []*mediaprovider.SearchResult{
		{Name: "Greatest Hits by The Rolling Stones", Type: mediaprovider.ContentTypeAlbum},
		{Name: "Rolling in the Deep", Type: mediaprovider.ContentTypeTrack},
		{Name: "The Rolling Stones", Type: mediaprovider.ContentTypeArtist},
		{Name: "Like a Rolling Stone", Type: mediaprovider.ContentTypeTrack},
	}

	fullQuery := "rolling stones"
	queryTerms := []string{"rolling", "stones"}

	RankSearchResults(results, fullQuery, queryTerms)

	// "The Rolling Stones" should be first (full query match, artist type)
	if results[0].Name != "The Rolling Stones" {
		t.Errorf("Expected 'The Rolling Stones' first, got %q", results[0].Name)
	}

	// "Greatest Hits by The Rolling Stones" should be second (contains full query)
	if results[1].Name != "Greatest Hits by The Rolling Stones" {
		t.Errorf("Expected 'Greatest Hits by The Rolling Stones' second, got %q", results[1].Name)
	}
}
