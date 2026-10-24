package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// rejectDuplicateJSONKeys checks every JSON object recursively.
// It rejects duplicate field names, including inside nested objects
// and objects contained in arrays.
func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))

	if err := scanUniqueJSONValue(decoder); err != nil {
		return err
	}

	_, err := decoder.Token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("invalid JSON after root value: %w", err)
	}

	return errors.New("multiple JSON values")
}

func scanUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON value: %w", err)
	}

	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delimiter {
	case '{':
		seen := make(map[string]struct{})

		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("invalid JSON object key: %w", err)
			}

			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key must be a string")
			}

			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}

			if err := scanUniqueJSONValue(decoder); err != nil {
				return err
			}
		}

		end, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON object ending: %w", err)
		}
		if end != json.Delim('}') {
			return errors.New("invalid JSON object ending")
		}

	case '[':
		for decoder.More() {
			if err := scanUniqueJSONValue(decoder); err != nil {
				return err
			}
		}

		end, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON array ending: %w", err)
		}
		if end != json.Delim(']') {
			return errors.New("invalid JSON array ending")
		}

	default:
		return errors.New("unexpected JSON delimiter")
	}

	return nil
}
