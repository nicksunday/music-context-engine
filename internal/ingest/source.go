package ingest

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type SourceKind string

const (
	SourceLastFM                    SourceKind = "lastfm"
	SourceYTMUploads                SourceKind = "youtube_music_uploads"
	SourceRYM                       SourceKind = "rateyourmusic"
	SourceAppleMusicLikes           SourceKind = "apple_music_likes"
	SourceAppleMusicFavorites       SourceKind = "apple_music_favorites"
	SourceAppleMusicLibraryActivity SourceKind = "apple_music_library_activity"
	SourceAppleMusicLibraryTracks   SourceKind = "apple_music_library_tracks"
	SourceAppleMusicPlayActivity    SourceKind = "apple_music_play_activity"
	SourceAppleMusicTrackHistory    SourceKind = "apple_music_track_play_history"
)

func ReadCSVHeader(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	firstByte, err := firstNonSpaceByte(file)
	if err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if firstByte == '[' || firstByte == '{' {
		return readJSONHeader(file)
	}

	reader := newCSVReader(file)

	header, err := reader.Read()
	if err != nil {
		return nil, err
	}

	return header, nil
}

func DetectSource(header []string) (SourceKind, error) {
	columns := headerColumns(header)

	switch {
	case hasHeaderPrefix(header, "Name", "Artist", "Composer", "Album"):
		return SourceAppleMusicLikes, nil
	case hasColumns(columns, "Favorite Type", "Item Description", "Preference"):
		return SourceAppleMusicFavorites, nil
	case hasColumns(columns, "Transaction Type", "Transaction Identifier", "Transaction Date"):
		return SourceAppleMusicLibraryActivity, nil
	case hasColumns(columns, "Track Identifier", "Title", "Content Type"):
		return SourceAppleMusicLibraryTracks, nil
	case isAppleMusicPlayActivityHeader(columns):
		return SourceAppleMusicPlayActivity, nil
	case hasColumns(columns, "Track Name", "Last Played Date", "Is User Initiated"):
		return SourceAppleMusicTrackHistory, nil
	case hasColumns(columns, "uts", "track_mbid", "artist"):
		return SourceLastFM, nil
	case hasColumns(columns, "Song Title", "Artist Name 1"):
		return SourceYTMUploads, nil
	case hasColumns(columns, "RYM Album ID", "Rating") || hasColumns(columns, "RYM Album", "Rating"):
		return SourceRYM, nil
	default:
		return "", fmt.Errorf("unrecognized CSV header signature: %s", strings.Join(header, ", "))
	}
}

func firstNonSpaceByte(file *os.File) (byte, error) {
	var buf [1]byte
	for {
		n, err := file.Read(buf[:])
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		if strings.TrimSpace(string(buf[0])) == "" {
			continue
		}
		return buf[0], nil
	}
}

func readJSONHeader(r io.Reader) ([]string, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()

	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}

	switch delimiter := token.(type) {
	case json.Delim:
		switch delimiter {
		case '[':
			if !decoder.More() {
				return nil, fmt.Errorf("empty JSON array has no source signature")
			}
			var object map[string]any
			if err := decoder.Decode(&object); err != nil {
				return nil, err
			}
			return objectKeys(object), nil
		case '{':
			var keys []string
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := token.(string)
				if !ok {
					return nil, fmt.Errorf("expected JSON object key token, got %T", token)
				}
				keys = append(keys, key)
				var value any
				if err := decoder.Decode(&value); err != nil {
					return nil, err
				}
			}
			return keys, nil
		default:
			return nil, fmt.Errorf("unsupported JSON delimiter %q", delimiter)
		}
	default:
		return nil, fmt.Errorf("unsupported JSON root token %T", token)
	}
}

func objectKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	return keys
}

func isAppleMusicPlayActivityHeader(columns map[string]int) bool {
	if !hasColumns(columns, "End Reason Type", "Play Duration Milliseconds") {
		return false
	}
	if _, ok := columns[headerKey("Event Timestamp")]; !ok {
		return false
	}
	if _, ok := columns[headerKey("Track Description")]; ok {
		return true
	}
	if _, ok := columns[headerKey("Song Name")]; ok {
		return true
	}
	return false
}

func newCSVReader(r io.Reader) *csv.Reader {
	reader := csv.NewReader(newCSVRecordSeparatorReader(r))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	return reader
}

type csvRecordSeparatorReader struct {
	r *bufio.Reader
}

func newCSVRecordSeparatorReader(r io.Reader) io.Reader {
	return &csvRecordSeparatorReader{r: bufio.NewReader(r)}
}

func (r *csvRecordSeparatorReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	n := 0
	for n < len(p) {
		b, err := r.r.ReadByte()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}

		if b == '\r' {
			if next, err := r.r.Peek(1); err == nil && next[0] == '\n' {
				_, _ = r.r.ReadByte()
			}
			b = '\n'
		}

		p[n] = b
		n++
	}

	return n, nil
}

func headerColumns(header []string) map[string]int {
	columns := make(map[string]int, len(header))
	for i, name := range header {
		columns[headerKey(name)] = i
	}
	return columns
}

func hasColumns(columns map[string]int, names ...string) bool {
	for _, name := range names {
		if _, ok := columns[headerKey(name)]; !ok {
			return false
		}
	}
	return true
}

func hasHeaderPrefix(header []string, names ...string) bool {
	if len(header) < len(names) {
		return false
	}
	for i, name := range names {
		if headerKey(header[i]) != headerKey(name) {
			return false
		}
	}
	return true
}

func requiredColumn(columns map[string]int, name string) (int, error) {
	idx, ok := columns[headerKey(name)]
	if !ok {
		return 0, fmt.Errorf("missing required column %q", name)
	}
	return idx, nil
}

func optionalColumn(columns map[string]int, names ...string) (int, bool) {
	for _, name := range names {
		if idx, ok := columns[headerKey(name)]; ok {
			return idx, true
		}
	}
	return 0, false
}

func headerKey(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))
}

func csvField(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}
