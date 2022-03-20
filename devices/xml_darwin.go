//go:build darwin
// +build darwin

package devices

import (
	"bytes"
	"encoding/xml"
	"golang.org/x/net/html/charset"
)

func parseXml(ctx Context, xmld []byte, parsed interface{}) error {
	reader := bytes.NewReader(xmld)
	decoder := xml.NewDecoder(reader)
	decoder.CharsetReader = charset.NewReaderLabel

	return decoder.Decode(parsed)
}
