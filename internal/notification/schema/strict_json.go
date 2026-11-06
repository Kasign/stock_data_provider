package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

var (
	ErrDuplicateJSONKey   = errors.New("notification: duplicate JSON object key")
	ErrInvalidJSONUnicode = errors.New("notification: invalid JSON Unicode escape")
)

func validateStrictJSON(raw []byte, maxDepth int) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("%w: input is not valid UTF-8", ErrInvalidEventJSON)
	}
	if err := validateUnicodeEscapes(raw); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder, 0, maxDepth); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("notification: multiple top-level JSON values")
		}
		return err
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder, depth, maxDepth int) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	depth++
	if depth > maxDepth {
		return ErrEventResourceLimitExceeded
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("notification: JSON object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("%w: %q", ErrDuplicateJSONKey, key)
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder, depth, maxDepth); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("notification: malformed JSON object")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder, depth, maxDepth); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("notification: malformed JSON array")
		}
	default:
		return errors.New("notification: unexpected JSON delimiter")
	}
	return nil
}

func validateUnicodeEscapes(raw []byte) error {
	inString := false
	for i := 0; i < len(raw); i++ {
		switch {
		case !inString && raw[i] == '"':
			inString = true
		case inString && raw[i] == '"':
			inString = false
		case inString && raw[i] == '\\':
			if i+1 >= len(raw) {
				return ErrInvalidJSONUnicode
			}
			if raw[i+1] != 'u' {
				i++
				continue
			}
			if i+5 >= len(raw) {
				return ErrInvalidJSONUnicode
			}
			code, ok := parseHexCodeUnit(raw[i+2 : i+6])
			if !ok {
				return ErrInvalidJSONUnicode
			}
			switch {
			case code >= 0xD800 && code <= 0xDBFF:
				if i+11 >= len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
					return ErrInvalidJSONUnicode
				}
				low, valid := parseHexCodeUnit(raw[i+8 : i+12])
				if !valid || low < 0xDC00 || low > 0xDFFF {
					return ErrInvalidJSONUnicode
				}
				i += 11
			case code >= 0xDC00 && code <= 0xDFFF:
				return ErrInvalidJSONUnicode
			default:
				i += 5
			}
		}
	}
	if inString {
		return ErrInvalidJSONUnicode
	}
	return nil
}

func parseHexCodeUnit(raw []byte) (uint16, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	var value uint16
	for _, b := range raw {
		value <<= 4
		switch {
		case b >= '0' && b <= '9':
			value |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			value |= uint16(b-'a') + 10
		case b >= 'A' && b <= 'F':
			value |= uint16(b-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

// ValidateStrictJSON applies the same duplicate-key, Unicode, UTF-8 and depth
// checks used by Event Registry before a consumer parses an event payload.
func ValidateStrictJSON(raw []byte, maxDepth int) error {
	if maxDepth <= 0 {
		return ErrInvalidResourceLimits
	}
	return validateStrictJSON(raw, maxDepth)
}
