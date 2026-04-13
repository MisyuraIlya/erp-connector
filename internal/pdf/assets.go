package pdf

import (
	_ "embed"
	"encoding/base64"
)

//go:embed templates/invoice.html
var invoiceTemplateHTML string

//go:embed fonts/NotoSansHebrew-VariableFont_wdth,wght.ttf
var notoSansHebrewFont []byte

// fontDataURI returns the NotoSansHebrew font as a base64 data URI for CSS @font-face.
func fontDataURI() string {
	return "data:font/ttf;base64," + base64.StdEncoding.EncodeToString(notoSansHebrewFont)
}
