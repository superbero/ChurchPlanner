package songs

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)

func RenderSongPDF(title, author, key, arrangement, sequence string, chart []RenderedSection, qrURL, language string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(18, 18, 18)
	pdf.SetAutoPageBreak(true, 15)

	if qrURL != "" {
		qrBytes, err := qrcode.Encode(qrURL, qrcode.Medium, 256)
		if err == nil {
			opts := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
			pdf.RegisterImageOptionsReader("qr", opts, bytes.NewReader(qrBytes))
			pdf.SetFooterFunc(func() {
				pdf.ImageOptions("qr", 185, 270, 15, 15, false, opts, 0, "")
				pdf.SetY(-14)
				pdf.SetFont("Helvetica", "I", 7)
				pdf.SetTextColor(120, 120, 120)
				pdf.CellFormat(0, 5, fmt.Sprintf("Scan to rehearse  ·  page %d", pdf.PageNo()), "", 0, "L", false, 0, "")
				pdf.SetTextColor(0, 0, 0)
			})
		}
	}

	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 18)
	pdf.CellFormat(0, 10, title, "", 1, "C", false, 0, "")

	pdf.SetFont("Helvetica", "I", 10)
	sub := author
	if arrangement != "" {
		sub += "  —  " + arrangement
	}
	if key != "" {
		sub += "  —  Key: " + key
	}
	if language != "" && language != "en" {
		sub += "  —  " + strings.ToUpper(language)
	}
	pdf.CellFormat(0, 6, sub, "", 1, "C", false, 0, "")

	if sequence != "" {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(0, 5, "Sequence: "+sequence, "", 1, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.Ln(4)

	const chordH = 4.0
	const lyricH = 5.0
	const lineGap = 2.0
	const sectionGap = 4.0

	for _, sec := range chart {
		if pdf.GetY() > 260 {
			pdf.AddPage()
		}
		if sec.Label != "" {
			pdf.SetFont("Helvetica", "B", 10)
			pdf.SetTextColor(60, 60, 60)
			pdf.CellFormat(0, 6, sec.Label, "", 1, "L", false, 0, "")
			pdf.SetTextColor(0, 0, 0)
		}

		charW := pdf.GetStringWidth("M") // approx for Courier 10

		for _, line := range sec.Lines {
			if pdf.GetY() > 265 {
				pdf.AddPage()
			}
			x0 := pdf.GetX()
			y0 := pdf.GetY()

			pdf.SetFont("Courier", "B", 9)
			for _, c := range line.Chords {
				pdf.SetXY(x0+float64(c.Position)*charW, y0)
				pdf.CellFormat(0, chordH, c.Chord, "", 0, "L", false, 0, "")
			}

			pdf.SetFont("Courier", "", 10)
			pdf.SetXY(x0, y0+chordH)
			pdf.CellFormat(0, lyricH, line.Lyric, "", 0, "L", false, 0, "")

			pdf.SetY(y0 + chordH + lyricH + lineGap)
		}
		pdf.Ln(sectionGap)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	return buf.Bytes(), nil
}
