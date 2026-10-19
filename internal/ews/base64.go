package ews

import (
	"bytes"
	"encoding/base64"
)

func decodeBase64(b []byte) ([]byte, error) {
	b = bytes.Join(bytes.Fields(b), nil)
	out := make([]byte, base64.StdEncoding.DecodedLen(len(b)))
	n, err := base64.StdEncoding.Decode(out, b)
	return out[:n], err
}
