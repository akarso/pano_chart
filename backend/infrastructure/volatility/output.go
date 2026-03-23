package volatility

import (
	"encoding/json"
	"fmt"
	"os"
)

// SaveToFile writes the aggregation result as pretty-printed JSON.
func SaveToFile(result Result, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encoding json: %w", err)
	}

	return nil
}
