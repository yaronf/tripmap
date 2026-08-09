package pdfarchive

import (
	_ "embed"

	"github.com/go-pdf/fpdf"
)

const fontFamily = "tripmap"

//go:embed fonts/DejaVuSans.ttf
var fontRegular []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var fontBold []byte

// registerFonts embeds DejaVu Sans (UTF-8) for body + bold text.
// AddUTF8FontFromBytes mutates its input, so each call gets a fresh copy.
func registerFonts(pdf *fpdf.Fpdf) {
	reg := append([]byte(nil), fontRegular...)
	bold := append([]byte(nil), fontBold...)
	pdf.AddUTF8FontFromBytes(fontFamily, "", reg)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", bold)
}
