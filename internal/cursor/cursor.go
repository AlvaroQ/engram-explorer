// Package cursor provides keyset-pagination cursor encoding and decoding.
//
// A cursor encodes a tuple (orderKey, id) as URL-safe base64 (unpadded,
// RawURLEncoding) of "orderKey|id". The split uses the LAST '|' so that
// orderKey values that themselves contain '|' round-trip correctly.
package cursor

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// Decoded holds the two fields extracted from a pagination cursor.
type Decoded struct {
	OrderKey string
	ID       int64
}

// Encode encodes (orderKey, id) into a URL-safe base64 cursor string.
// Uses RawURLEncoding (no padding) to match the Node base64url encoding.
func Encode(orderKey string, id int64) string {
	raw := orderKey + "|" + strconv.FormatInt(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode decodes a cursor string and returns the extracted fields.
// Returns nil if the cursor is invalid (bad base64, missing '|', non-integer id).
func Decode(cursor string) *Decoded {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil
	}
	raw := string(b)
	sepIdx := strings.LastIndex(raw, "|")
	if sepIdx < 0 {
		return nil
	}
	orderKey := raw[:sepIdx]
	idStr := raw[sepIdx+1:]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil
	}
	return &Decoded{OrderKey: orderKey, ID: id}
}
