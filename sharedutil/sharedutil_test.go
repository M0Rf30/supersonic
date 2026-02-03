package sharedutil

import (
	"slices"
	"testing"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
)

func TestFilterSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		filter   func(int) bool
		expected []int
	}{
		{
			name:     "filter even numbers",
			input:    []int{1, 2, 3, 4, 5, 6},
			filter:   func(n int) bool { return n%2 == 0 },
			expected: []int{2, 4, 6},
		},
		{
			name:     "filter nothing",
			input:    []int{1, 2, 3},
			filter:   func(n int) bool { return true },
			expected: []int{1, 2, 3},
		},
		{
			name:     "filter everything",
			input:    []int{1, 2, 3},
			filter:   func(n int) bool { return false },
			expected: []int{},
		},
		{
			name:     "empty slice",
			input:    []int{},
			filter:   func(n int) bool { return true },
			expected: []int{},
		},
		{
			name:     "nil slice",
			input:    nil,
			filter:   func(n int) bool { return true },
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterSlice(tt.input, tt.filter)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("FilterSlice() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMapSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		mapper   func(int) string
		expected []string
	}{
		{
			name:     "int to string",
			input:    []int{1, 2, 3},
			mapper:   func(n int) string { return string(rune('a' + n - 1)) },
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty slice",
			input:    []int{},
			mapper:   func(n int) string { return "" },
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			mapper:   func(n int) string { return "" },
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapSlice(tt.input, tt.mapper)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("MapSlice() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterMapSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		mapper   func(int) (string, bool)
		expected []string
	}{
		{
			name:  "map and filter even numbers",
			input: []int{1, 2, 3, 4, 5},
			mapper: func(n int) (string, bool) {
				if n%2 == 0 {
					return string(rune('a' + n - 1)), true
				}
				return "", false
			},
			expected: []string{"b", "d"},
		},
		{
			name:     "filter all out",
			input:    []int{1, 2, 3},
			mapper:   func(n int) (string, bool) { return "", false },
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			mapper:   func(n int) (string, bool) { return "", true },
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterMapSlice(tt.input, tt.mapper)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("FilterMapSlice() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestReversed(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			name:     "reverse numbers",
			input:    []int{1, 2, 3, 4, 5},
			expected: []int{5, 4, 3, 2, 1},
		},
		{
			name:     "single element",
			input:    []int{1},
			expected: []int{1},
		},
		{
			name:     "empty slice",
			input:    []int{},
			expected: []int{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Reversed(tt.input)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("Reversed() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestToSet(t *testing.T) {
	tests := []struct {
		name          string
		input         []string
		expectedLen   int
		shouldContain []string
	}{
		{
			name:          "unique strings",
			input:         []string{"a", "b", "c"},
			expectedLen:   3,
			shouldContain: []string{"a", "b", "c"},
		},
		{
			name:          "with duplicates",
			input:         []string{"a", "b", "a", "c", "b"},
			expectedLen:   3,
			shouldContain: []string{"a", "b", "c"},
		},
		{
			name:          "empty slice",
			input:         []string{},
			expectedLen:   0,
			shouldContain: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToSet(tt.input)
			if len(result) != tt.expectedLen {
				t.Errorf("ToSet() len = %d, want %d", len(result), tt.expectedLen)
			}
			for _, item := range tt.shouldContain {
				if _, ok := result[item]; !ok {
					t.Errorf("ToSet() missing expected item: %s", item)
				}
			}
		})
	}
}

func TestFindTrackByID(t *testing.T) {
	tracks := []*mediaprovider.Track{
		{ID: "track1", Title: "Song 1"},
		{ID: "track2", Title: "Song 2"},
		{ID: "track3", Title: "Song 3"},
	}

	tests := []struct {
		name     string
		id       string
		expected *mediaprovider.Track
	}{
		{
			name:     "find existing track",
			id:       "track2",
			expected: tracks[1],
		},
		{
			name:     "track not found",
			id:       "track999",
			expected: nil,
		},
		{
			name:     "empty id",
			id:       "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindTrackByID(tt.id, tracks)
			if result != tt.expected {
				t.Errorf("FindTrackByID() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMediaItemIDOrEmptyStr(t *testing.T) {
	track := &mediaprovider.Track{ID: "track123"}
	radio := &mediaprovider.RadioStation{ID: "radio456"}

	tests := []struct {
		name     string
		item     mediaprovider.MediaItem
		expected string
	}{
		{
			name:     "track item",
			item:     track,
			expected: "track123",
		},
		{
			name:     "radio item",
			item:     radio,
			expected: "radio456",
		},
		{
			name:     "nil track",
			item:     (*mediaprovider.Track)(nil),
			expected: "",
		},
		{
			name:     "nil radio",
			item:     (*mediaprovider.RadioStation)(nil),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MediaItemIDOrEmptyStr(tt.item)
			if result != tt.expected {
				t.Errorf("MediaItemIDOrEmptyStr() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAlbumIDOrEmptyStr(t *testing.T) {
	tests := []struct {
		name     string
		track    *mediaprovider.Track
		expected string
	}{
		{
			name:     "track with album",
			track:    &mediaprovider.Track{AlbumID: "album123"},
			expected: "album123",
		},
		{
			name:     "nil track",
			track:    nil,
			expected: "",
		},
		{
			name:     "track without album",
			track:    &mediaprovider.Track{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AlbumIDOrEmptyStr(tt.track)
			if result != tt.expected {
				t.Errorf("AlbumIDOrEmptyStr() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTracksToIDs(t *testing.T) {
	tests := []struct {
		name     string
		tracks   []*mediaprovider.Track
		expected []string
	}{
		{
			name: "multiple tracks",
			tracks: []*mediaprovider.Track{
				{ID: "id1"},
				{ID: "id2"},
				{ID: "id3"},
			},
			expected: []string{"id1", "id2", "id3"},
		},
		{
			name:     "empty slice",
			tracks:   []*mediaprovider.Track{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			tracks:   nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TracksToIDs(tt.tracks)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("TracksToIDs() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func Test_ReorderItems(t *testing.T) {
	tracks := []*mediaprovider.Track{
		{ID: "a"}, // 0
		{ID: "b"}, // 1
		{ID: "c"}, // 2
		{ID: "d"}, // 3
		{ID: "e"}, // 4
		{ID: "f"}, // 5
	}

	// test MoveToTop:
	idxToMove := []int{0, 2, 3, 5}
	want := []*mediaprovider.Track{
		{ID: "a"},
		{ID: "c"},
		{ID: "d"},
		{ID: "f"},
		{ID: "b"},
		{ID: "e"},
	}
	newTracks := ReorderItems(tracks, idxToMove, 0)
	if !tracklistsEqual(t, newTracks, want) {
		t.Error("ReorderTracks: MoveToTop order incorrect")
	}

	// test MoveToBottom:
	idxToMove = []int{0, 2, 5}
	want = []*mediaprovider.Track{
		{ID: "b"},
		{ID: "d"},
		{ID: "e"},
		{ID: "a"},
		{ID: "c"},
		{ID: "f"},
	}
	newTracks = ReorderItems(tracks, idxToMove, len(tracks))
	if !tracklistsEqual(t, newTracks, want) {
		t.Error("ReorderTracks: MoveToBottom order incorrect")
	}
}

func tracklistsEqual(t *testing.T, a, b []*mediaprovider.Track) bool {
	t.Helper()
	return slices.EqualFunc(a, b, func(a, b *mediaprovider.Track) bool {
		return a.ID == b.ID
	})
}
