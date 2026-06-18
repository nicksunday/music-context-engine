package ingest

import "testing"

func TestDetectSource(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		want   SourceKind
	}{
		{
			name:   "lastfm",
			header: []string{"uts", "utc_time", "artist", "track", "track_mbid"},
			want:   SourceLastFM,
		},
		{
			name:   "youtube music uploads",
			header: []string{"Song Title", "Album Title", "Artist Name 1"},
			want:   SourceYTMUploads,
		},
		{
			name:   "rateyourmusic album id",
			header: []string{"RYM Album ID", "Title", "Rating"},
			want:   SourceRYM,
		},
		{
			name:   "rateyourmusic legacy album id",
			header: []string{"RYM Album", "Title", "Rating"},
			want:   SourceRYM,
		},
		{
			name:   "apple music likes",
			header: []string{"Name", "Artist", "Composer", "Album", "Kind"},
			want:   SourceAppleMusicLikes,
		},
		{
			name:   "apple music favorites",
			header: []string{"Favorite Type", "Item Reference", "Item Description", "Last Modified", "Preference"},
			want:   SourceAppleMusicFavorites,
		},
		{
			name:   "apple music library activity",
			header: []string{"Transaction Type", "Transaction Identifier", "Transaction Date", "UserAgent"},
			want:   SourceAppleMusicLibraryActivity,
		},
		{
			name:   "apple music library tracks",
			header: []string{"Content Type", "Track Identifier", "Title", "Artist", "Album"},
			want:   SourceAppleMusicLibraryTracks,
		},
		{
			name:   "apple music play activity",
			header: []string{"Event Timestamp", "Song Name", "Play Duration Milliseconds", "End Reason Type"},
			want:   SourceAppleMusicPlayActivity,
		},
		{
			name:   "apple music track play history",
			header: []string{"Track Name", "Last Played Date", "Is User Initiated"},
			want:   SourceAppleMusicTrackHistory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectSource(tt.header)
			if err != nil {
				t.Fatalf("DetectSource() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("DetectSource() = %q, want %q", got, tt.want)
			}
		})
	}
}
