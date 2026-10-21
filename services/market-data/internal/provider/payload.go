package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const maxProviderPayloadBytes int64 = 8 << 20

func readBoundedBody(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("payload size limit must be positive")
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read provider payload: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("provider payload exceeds %d byte limit", maxBytes)
	}
	return body, nil
}

func decodeSingleJSON(body []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("provider payload contains multiple JSON values")
		}
		return fmt.Errorf("provider payload has trailing non-JSON content: %w", err)
	}
	return nil
}
