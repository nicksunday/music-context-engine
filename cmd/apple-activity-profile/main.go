package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultActivityDir = "data/Apple Music Activity"

func main() {
	activityDir := defaultActivityDir
	if len(os.Args) > 1 {
		activityDir = os.Args[1]
	}

	if err := run(activityDir); err != nil {
		fmt.Fprintf(os.Stderr, "apple activity profile failed: %v\n", err)
		os.Exit(1)
	}
}

func run(activityDir string) error {
	tracksPath := filepath.Join(activityDir, "Apple Music Library Tracks.json")
	playPath := filepath.Join(activityDir, "Apple Music Play Activity.csv")
	favoritesPath := filepath.Join(activityDir, "Apple Music - Favorites.csv")

	fmt.Printf("Apple Music activity directory: %s\n\n", activityDir)

	if err := profileLibraryTracks(tracksPath); err != nil {
		return err
	}
	fmt.Println()

	playRows, playHeaders, err := profileCSV(playPath)
	if err != nil {
		return err
	}
	fmt.Printf("Play activity CSV: %s\n", playPath)
	fmt.Printf("Data rows: %d\n", playRows)
	fmt.Printf("Column headers (%d):\n", len(playHeaders))
	for i, header := range playHeaders {
		fmt.Printf("  %02d. %s\n", i+1, header)
	}
	fmt.Println()

	favoriteRows, favoriteHeaders, explicitLikes, err := profileFavorites(favoritesPath)
	if err != nil {
		return err
	}
	fmt.Printf("Favorites CSV: %s\n", favoritesPath)
	fmt.Printf("Data rows: %d\n", favoriteRows)
	fmt.Printf("Column headers (%d): %s\n", len(favoriteHeaders), strings.Join(favoriteHeaders, ", "))
	fmt.Printf("Explicit LIKE entries: %d\n", explicitLikes)

	return nil
}

func profileLibraryTracks(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	fmt.Printf("Library tracks JSON: %s\n", path)
	switch delimiter := token.(type) {
	case json.Delim:
		switch delimiter {
		case '[':
			fmt.Println("Top-level type: array")
			if !decoder.More() {
				fmt.Println("Array items: 0")
				return nil
			}

			var sample map[string]any
			if err := decoder.Decode(&sample); err != nil {
				return err
			}
			stats := newFieldStats()
			stats.add(sample)
			count := 1
			for decoder.More() {
				var object map[string]any
				if err := decoder.Decode(&object); err != nil {
					return err
				}
				stats.add(object)
				count++
			}
			if _, err := decoder.Token(); err != nil {
				return err
			}

			fmt.Println("Top-level keys: none (bare JSON array)")
			fmt.Printf("Array items: %d\n", count)
			stats.print("Observed track fields across all items", count)
			printObjectStructure("Sample track object structure", sample)
		case '{':
			fmt.Println("Top-level type: object")
			keys, sample, err := profileObjectWrappedTracks(decoder)
			if err != nil {
				return err
			}
			fmt.Printf("Top-level keys (%d): %s\n", len(keys), strings.Join(keys, ", "))
			if sample != nil {
				printObjectStructure("Sample track object structure", sample)
			}
		default:
			return fmt.Errorf("unexpected top-level JSON delimiter %q", delimiter)
		}
	default:
		return fmt.Errorf("unexpected top-level JSON token %T", token)
	}

	return nil
}

func profileObjectWrappedTracks(decoder *json.Decoder) ([]string, map[string]any, error) {
	var keys []string
	var sample map[string]any

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected object key token, got %T", token)
		}
		keys = append(keys, key)

		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
		if sample == nil {
			sample = firstObject(value)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, nil, err
	}
	sort.Strings(keys)

	return keys, sample, nil
}

func firstObject(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case []any:
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				return object
			}
		}
	}
	return nil
}

func printObjectStructure(title string, object map[string]any) {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fmt.Printf("%s (%d fields):\n", title, len(keys))
	for _, key := range keys {
		fmt.Printf("  - %s: %s\n", key, describeValue(object[key]))
	}
}

func describeValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("string%s", sampleSuffix(typed))
	case bool:
		return fmt.Sprintf("bool (%t)", typed)
	case json.Number:
		return fmt.Sprintf("number (%s)", typed.String())
	case []any:
		return fmt.Sprintf("array[%d]", len(typed))
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return fmt.Sprintf("object{%s}", strings.Join(keys, ", "))
	default:
		return fmt.Sprintf("%T", value)
	}
}

func sampleSuffix(value string) string {
	const maxLen = 72
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\r")
	if value == "" {
		return " (empty)"
	}
	if len(value) > maxLen {
		value = value[:maxLen] + "..."
	}
	return fmt.Sprintf(" (%q)", value)
}

type fieldStats map[string]*fieldStat

type fieldStat struct {
	count int
	types map[string]int
}

func newFieldStats() fieldStats {
	return make(fieldStats)
}

func (stats fieldStats) add(object map[string]any) {
	for key, value := range object {
		stat := stats[key]
		if stat == nil {
			stat = &fieldStat{types: make(map[string]int)}
			stats[key] = stat
		}
		stat.count++
		stat.types[typeName(value)]++
	}
}

func (stats fieldStats) print(title string, total int) {
	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fmt.Printf("%s (%d fields):\n", title, len(keys))
	for _, key := range keys {
		stat := stats[key]
		fmt.Printf("  - %s: present %d/%d, types %s\n", key, stat.count, total, formatTypeCounts(stat.types))
	}
}

func typeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func formatTypeCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

func profileCSV(path string) (int, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	headers, err := reader.Read()
	if err != nil {
		return 0, nil, err
	}

	rows := 0
	for {
		if _, err := reader.Read(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, nil, err
		}
		rows++
	}

	return rows, headers, nil
}

func profileFavorites(path string) (int, []string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, nil, 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	headers, err := reader.Read()
	if err != nil {
		return 0, nil, 0, err
	}

	preferenceIndex := -1
	for index, header := range headers {
		if strings.EqualFold(strings.TrimSpace(header), "Preference") {
			preferenceIndex = index
			break
		}
	}
	if preferenceIndex == -1 {
		return 0, nil, 0, fmt.Errorf("%s is missing Preference column", path)
	}

	rows := 0
	likes := 0
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, nil, 0, err
		}
		rows++
		if preferenceIndex < len(record) && strings.EqualFold(strings.TrimSpace(record[preferenceIndex]), "LIKE") {
			likes++
		}
	}

	return rows, headers, likes, nil
}
