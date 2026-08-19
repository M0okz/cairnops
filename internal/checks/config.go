package checks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func decodeConfig(raw json.RawMessage, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode config: trailing data")
	}
	return nil
}
