package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"pano_chart/backend/domain/scoring"
)

const reservedSectorOther = "other"

// SectorDef is one configured sector (id, display name, symbols).
type SectorDef struct {
	ID      string
	Name    string
	Symbols []string
}

// SectorCatalog maps each symbol to exactly one sector.
// Unlisted symbols resolve to id "other" via ForSymbol.
type SectorCatalog struct {
	sectors  []SectorDef
	bySymbol map[string]string // symbol → sector id (exclusive membership)
}

type sectorsYAML struct {
	Sectors []struct {
		ID      string   `yaml:"id"`
		Name    string   `yaml:"name"`
		Symbols []string `yaml:"symbols"`
	} `yaml:"sectors"`
}

// LoadSectorCatalog parses sectors.yaml.
// Rejects empty id/name entries, duplicate ids, reserved id "other", and
// symbols listed in more than one sector.
func LoadSectorCatalog(path string) (*SectorCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sectors config %s: %w", path, err)
	}
	var raw sectorsYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse sectors config %s: %w", path, err)
	}
	if len(raw.Sectors) == 0 {
		return nil, fmt.Errorf("sectors config %s: no sectors defined", path)
	}

	cat := &SectorCatalog{
		sectors:  make([]SectorDef, 0, len(raw.Sectors)),
		bySymbol: make(map[string]string),
	}
	seenID := make(map[string]struct{}, len(raw.Sectors))

	for _, s := range raw.Sectors {
		id, idErr := NormalizeSectorID(s.ID)
		name := strings.TrimSpace(s.Name)
		if idErr != nil || name == "" {
			if idErr != nil {
				return nil, fmt.Errorf("sectors config %s: %w", path, idErr)
			}
			return nil, fmt.Errorf("sectors config %s: sector missing id or name", path)
		}
		if id == reservedSectorOther {
			return nil, fmt.Errorf("sectors config %s: id %q is reserved", path, reservedSectorOther)
		}
		if _, dup := seenID[id]; dup {
			return nil, fmt.Errorf("sectors config %s: duplicate sector id %q", path, id)
		}
		seenID[id] = struct{}{}

		syms := make([]string, 0, len(s.Symbols))
		seenInSector := make(map[string]struct{}, len(s.Symbols))
		for _, sym := range s.Symbols {
			sym = strings.ToUpper(strings.TrimSpace(sym))
			if sym == "" {
				continue
			}
			if _, dup := seenInSector[sym]; dup {
				continue
			}
			if prev, ok := cat.bySymbol[sym]; ok {
				return nil, fmt.Errorf(
					"sectors config %s: symbol %s listed in both %q and %q",
					path, sym, prev, id,
				)
			}
			seenInSector[sym] = struct{}{}
			cat.bySymbol[sym] = id
			syms = append(syms, sym)
		}
		if len(syms) == 0 {
			return nil, fmt.Errorf("sectors config %s: sector %q has no symbols", path, id)
		}
		cat.sectors = append(cat.sectors, SectorDef{ID: id, Name: name, Symbols: syms})
	}
	return cat, nil
}

// SectorsPath resolves sectors.yaml. Prefer $SECTORS_CONFIG_PATH; otherwise
// config/sectors.yaml next to scoring.ConfigPath()'s directory.
func SectorsPath() string {
	if p := os.Getenv("SECTORS_CONFIG_PATH"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(scoring.ConfigPath()), "config", "sectors.yaml")
}

// Sectors returns configured sectors in YAML order (excludes synthetic "other").
func (c *SectorCatalog) Sectors() []SectorDef {
	if c == nil {
		return nil
	}
	out := make([]SectorDef, len(c.sectors))
	copy(out, c.sectors)
	return out
}

// ForSymbol returns the sector id for sym, or "other" when unlisted.
func (c *SectorCatalog) ForSymbol(sym string) string {
	if c == nil {
		return reservedSectorOther
	}
	sym = strings.ToUpper(strings.TrimSpace(sym))
	if id, ok := c.bySymbol[sym]; ok {
		return id
	}
	return reservedSectorOther
}

// Members returns in-universe symbols assigned to sectorID via bySymbol.
func (c *SectorCatalog) Members(sectorID string, universe []string) []string {
	if c == nil {
		return nil
	}
	id, err := NormalizeSectorID(sectorID)
	if err != nil {
		return nil
	}
	out := make([]string, 0)
	for _, sym := range universe {
		if c.ForSymbol(sym) == id {
			out = append(out, sym)
		}
	}
	return out
}
