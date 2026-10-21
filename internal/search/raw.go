package search

import (
	"encoding/json"
	"fmt"
	"io"
)

// maxRawBodyBytes caps how much of a search provider response is buffered for
// raw debugging output. Search responses are small JSON documents; a larger
// body almost certainly means an unexpected error page.
const maxRawBodyBytes = 10 << 20

// readRawBody buffers a provider response so it can be both decoded into the
// normalized result shape and returned verbatim for `--output-json`.
func readRawBody(r io.Reader) (json.RawMessage, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxRawBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRawBodyBytes {
		return nil, fmt.Errorf("response too large: %d MB", len(data)/(1<<20))
	}
	return json.RawMessage(data), nil
}
