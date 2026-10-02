package metrics

import (
	"fmt"
	"strings"
)

// NormalizeSectorID lowercases and validates a sector id for use in catalog
// keys and seasonality filenames. Allowed: [a-z0-9_-]. Rejects empty ids
// and path separators.
func NormalizeSectorID(id string) (string, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return "", fmt.Errorf("empty sector id")
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return "", fmt.Errorf("invalid sector id %q: disallowed character %q", id, c)
		}
	}
	return id, nil
}

// SectorProfilePath joins a path stem with a sector id.
// Prefix is the stem without a trailing underscore (e.g. "/data/vol");
// the result is "/data/vol_l1.json". Writer (--out) and reader
// (VOL_SECTOR_PREFIX) must use the same stem.
func SectorProfilePath(prefix, sectorID string) (string, error) {
	id, err := NormalizeSectorID(sectorID)
	if err != nil {
		return "", err
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", fmt.Errorf("empty sector profile prefix")
	}
	return prefix + "_" + id + ".json", nil
}
