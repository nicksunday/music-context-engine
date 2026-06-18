package ingest

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func trackID(title, artist, album string) string {
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s-%s-%s", title, artist, album)))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func albumID(cleanTitle, cleanArtist string) string {
	key := strings.TrimSpace(cleanTitle) + "-" + strings.TrimSpace(cleanArtist)
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum)
}

func appleMusicPlayActivityID(eventTimestamp, trackDescription, playDuration, endReason string) string {
	key := strings.Join([]string{
		strings.TrimSpace(eventTimestamp),
		strings.TrimSpace(trackDescription),
		strings.TrimSpace(playDuration),
		strings.TrimSpace(endReason),
	}, "-")
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum)
}
