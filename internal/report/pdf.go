package report

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"strings"

	"github.com/signintech/gopdf"
)

// Inter, by The Inter Project Authors, under the SIL Open Font License
// (fonts/OFL.txt). Only the glyphs a report uses are embedded in it.
var (
	//go:embed fonts/Inter-Regular.ttf
	fontRegular []byte
	//go:embed fonts/Inter-Bold.ttf
	fontBold []byte
)

// A4 portrait, in points.
const (
	pageW    = 595.28
	pageH    = 841.89
	margin   = 40.0
	contentW = pageW - 2*margin
	footerY  = pageH - 28
	bottom   = footerY - 14 // content ends here
)

type rgb struct{ r, g, b uint8 }

// The dashboard's palette.
var (
	colNavy     = rgb{7, 16, 24}
	colOrange   = rgb{255, 122, 69}
	colText     = rgb{31, 41, 55}
	colMuted    = rgb{107, 114, 128}
	colLine     = rgb{229, 231, 235}
	colTile     = rgb{246, 247, 249}
	colOK       = rgb{22, 163, 74}
	colWarn     = rgb{217, 119, 6}
	colFail     = rgb{220, 38, 38}
	colHeaderBG = rgb{241, 243, 246}
)

// tone is the look of a status: its color and a light background.
type tone struct{ fg, bg rgb }

var (
	toneOK      = tone{rgb{22, 101, 52}, rgb{220, 252, 231}}
	toneWarn    = tone{rgb{146, 64, 14}, rgb{254, 243, 199}}
	toneFail    = tone{rgb{153, 27, 27}, rgb{254, 226, 226}}
	toneNeutral = tone{rgb{55, 65, 81}, rgb{229, 231, 235}}
)

const (
	fontR = "Inter"
	fontB = "Inter-Bold"
)

// doc is a report being drawn, top to bottom.
type doc struct {
	pdf   *gopdf.GoPdf
	in    Input
	title string
	y     float64
	err   error
}

func newDoc(in Input, title string) (*doc, error) {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.SetCompressLevel(9)
	if err := pdf.AddTTFFontData(fontR, fontRegular); err != nil {
		return nil, err
	}
	if err := pdf.AddTTFFontData(fontB, fontBold); err != nil {
		return nil, err
	}
	pdf.SetInfo(gopdf.PdfInfo{Title: title, Author: "HitStor", Creator: "HitStor " + in.Version, CreationDate: in.Generated})
	d := &doc{pdf: pdf, in: in, title: title}
	d.newPage(true)
	return d, nil
}

func (d *doc) check(err error) {
	if err != nil && d.err == nil {
		d.err = err
	}
}

func (d *doc) font(bold bool, size float64) {
	name := fontR
	if bold {
		name = fontB
	}
	d.check(d.pdf.SetFont(name, "", size))
}

func (d *doc) color(c rgb)  { d.pdf.SetTextColor(c.r, c.g, c.b) }
func (d *doc) fill(c rgb)   { d.pdf.SetFillColor(c.r, c.g, c.b) }
func (d *doc) stroke(c rgb) { d.pdf.SetStrokeColor(c.r, c.g, c.b) }

func (d *doc) width(s string) float64 {
	w, err := d.pdf.MeasureTextWidth(s)
	d.check(err)
	return w
}

// text writes s with its top-left corner at x, y.
func (d *doc) text(x, y float64, s string, bold bool, size float64, c rgb) {
	d.font(bold, size)
	d.color(c)
	d.pdf.SetXY(x, y)
	d.check(d.pdf.CellWithOption(&gopdf.Rect{W: d.width(s) + 1, H: size * 1.25}, s, gopdf.CellOption{Align: gopdf.Left | gopdf.Top}))
}

// textRight writes s ending at x.
func (d *doc) textRight(x, y float64, s string, bold bool, size float64, c rgb) {
	d.font(bold, size)
	d.text(x-d.width(s), y, s, bold, size, c)
}

// wrap splits s into lines no wider than w.
func (d *doc) wrap(s string, bold bool, size, w float64) []string {
	d.font(bold, size)
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		lines, err := d.pdf.SplitTextWithWordWrap(para, w)
		if err != nil {
			// A word wider than the line: break it by characters.
			lines, err = d.pdf.SplitText(para, w)
			d.check(err)
		}
		out = append(out, lines...)
	}
	return out
}

func (d *doc) rect(x, y, w, h float64, c rgb, radius float64) {
	d.fill(c)
	if radius > 0 {
		d.check(d.pdf.Rectangle(x, y, x+w, y+h, "F", radius, 8))
		return
	}
	d.pdf.RectFromUpperLeftWithStyle(x, y, w, h, "F")
}

func (d *doc) hline(x1, x2, y float64, c rgb, width float64) {
	d.stroke(c)
	d.pdf.SetLineWidth(width)
	d.pdf.Line(x1, y, x2, y)
}

// newPage starts a page with the report's header.
func (d *doc) newPage(first bool) {
	d.pdf.AddPage()
	h := 34.0
	if first {
		h = 84
	}
	d.rect(0, 0, pageW, h, colNavy, 0)
	logo := 0.0
	if first && len(d.in.Logo) > 0 {
		if img, _, err := image.Decode(bytes.NewReader(d.in.Logo)); err == nil {
			d.check(d.pdf.ImageFrom(img, margin, 22, &gopdf.Rect{W: 40, H: 40}))
			logo = 50
		}
	}
	if first {
		d.text(margin+logo, 28, "hit", true, 26, rgb{244, 247, 251})
		d.font(true, 26)
		d.text(margin+logo+d.width("hit"), 28, "stor", true, 26, colOrange)
		d.textRight(pageW-margin, 24, d.title, true, 16, rgb{244, 247, 251})
		d.textRight(pageW-margin, 46, d.periodText(), false, 10, rgb{190, 199, 212})
		if scope := d.scopeText(); scope != "" {
			d.textRight(pageW-margin, 61, scope, false, 8.5, rgb{150, 160, 175})
		}
		d.y = h + 22
		return
	}
	d.text(margin, 10, "hit", true, 12, rgb{244, 247, 251})
	d.font(true, 12)
	d.text(margin+d.width("hit"), 10, "stor", true, 12, colOrange)
	d.textRight(pageW-margin, 11, d.title+" · "+d.periodText(), false, 9, rgb{190, 199, 212})
	d.y = h + 20
}

func (d *doc) periodText() string {
	in := d.in
	if in.Type == TypeDaily {
		return in.formatTime(in.From) + " – " + in.formatTime(in.To)
	}
	return in.formatDate(in.From) + " – " + in.formatDate(in.To.Add(-1))
}

func (d *doc) scopeText() string {
	if len(d.in.Scope) == 0 {
		return ""
	}
	s := strings.Join(d.in.Scope, ", ")
	if r := []rune(s); len(r) > 70 {
		s = string(r[:67]) + "…"
	}
	return d.in.Catalog.Tf("Agents: {list}", "list", s)
}

// room makes sure h points fit on the page, starting a new one if not.
func (d *doc) room(h float64) {
	if d.y+h > bottom {
		d.newPage(false)
	}
}

func (d *doc) gap(h float64) { d.y += h }

// heading starts a section, with the space its first lines need.
func (d *doc) heading(s string) {
	d.room(60)
	d.gap(6)
	d.text(margin, d.y, s, true, 13, colText)
	d.y += 20
}

// paragraph writes wrapped text.
func (d *doc) paragraph(s string, size float64, c rgb) {
	lh := size * 1.45
	for _, line := range d.wrap(s, false, size, contentW) {
		d.room(lh)
		d.text(margin, d.y, line, false, size, c)
		d.y += lh
	}
}

// banner is the report's verdict: a colored bar, a title and a reason.
func (d *doc) banner(level, title, detail string) {
	t := map[string]tone{LevelGood: toneOK, LevelAttention: toneWarn, LevelCritical: toneFail}[level]
	accent := map[string]rgb{LevelGood: colOK, LevelAttention: colWarn, LevelCritical: colFail}[level]
	lines := d.wrap(detail, false, 9.5, contentW-36)
	h := 30 + float64(len(lines))*13.5
	d.room(h)
	d.rect(margin, d.y, contentW, h, t.bg, 6)
	d.rect(margin, d.y, 5, h, accent, 0)
	d.text(margin+18, d.y+10, title, true, 13, t.fg)
	for i, line := range lines {
		d.text(margin+18, d.y+28+float64(i)*13.5, line, false, 9.5, t.fg)
	}
	d.y += h + 16
}

// kpi is a figure shown in a tile.
type kpi struct {
	value, label, note string
	c                  rgb
}

// tiles draws up to four figures side by side.
func (d *doc) tiles(ks []kpi) {
	const h, gapX = 64.0, 10.0
	d.room(h)
	w := (contentW - gapX*float64(len(ks)-1)) / float64(len(ks))
	for i, k := range ks {
		x := margin + float64(i)*(w+gapX)
		d.rect(x, d.y, w, h, colTile, 6)
		d.text(x+12, d.y+10, k.value, true, 19, k.c)
		d.text(x+12, d.y+35, k.label, true, 8.5, colText)
		if k.note != "" {
			note := k.note
			d.font(false, 7.5)
			for d.width(note) > w-20 && len([]rune(note)) > 4 {
				r := []rune(note)
				note = string(r[:len(r)-2]) + "…"
			}
			d.text(x+12, d.y+48, note, false, 7.5, colMuted)
		}
	}
	d.y += h + 16
}

// chip draws a status label at x, y and returns its width.
func (d *doc) chip(x, y float64, s string, t tone) float64 {
	d.font(true, 7.5)
	w := d.width(s) + 12
	d.rect(x, y, w, 13, t.bg, 6.5)
	d.text(x+6, y+2.6, s, true, 7.5, t.fg)
	return w
}

// column of a table.
type column struct {
	title string
	w     float64 // share of the width; the shares add up to 1
	right bool
}

// cell of a table: text, or a status chip.
type cell struct {
	text  string
	bold  bool
	muted bool
	chip  *tone
}

func chipCell(s string, t tone) cell { return cell{text: s, chip: &t} }

// table draws rows under a header, repeating it on each new page; text
// wraps within its column.
func (d *doc) table(cols []column, rows [][]cell) {
	const size, lh, pad = 8.5, 11.5, 6.0
	header := func() {
		d.rect(margin, d.y, contentW, 20, colHeaderBG, 3)
		x := margin
		for _, c := range cols {
			w := c.w * contentW
			if c.right {
				d.textRight(x+w-pad, d.y+6, c.title, true, 7.5, colMuted)
			} else {
				d.text(x+pad, d.y+6, c.title, true, 7.5, colMuted)
			}
			x += w
		}
		d.y += 20
	}
	d.room(20 + lh + 2*pad)
	header()
	for _, row := range rows {
		wrapped := make([][]string, len(row))
		lines := 1
		for i, c := range row {
			if c.chip != nil {
				continue
			}
			wrapped[i] = d.wrap(c.text, c.bold, size, cols[i].w*contentW-2*pad)
			if len(wrapped[i]) > lines {
				lines = len(wrapped[i])
			}
		}
		h := float64(lines)*lh + 2*pad - 2
		if d.y+h > bottom {
			d.newPage(false)
			header()
		}
		x := margin
		for i, c := range row {
			w := cols[i].w * contentW
			color := colText
			if c.muted {
				color = colMuted
			}
			switch {
			case c.chip != nil:
				d.chip(x+pad, d.y+pad-1.5, c.text, *c.chip)
			default:
				for li, line := range wrapped[i] {
					ly := d.y + pad + float64(li)*lh - 1
					if cols[i].right {
						d.textRight(x+w-pad, ly, line, c.bold, size, color)
					} else {
						d.text(x+pad, ly, line, c.bold, size, color)
					}
				}
			}
			x += w
		}
		d.y += h
		d.hline(margin, margin+contentW, d.y, colLine, 0.6)
	}
	d.y += 16
}

// bar is one day of the chart.
type bar struct {
	label             string
	ok, partial, fail int
}

// barChart draws stacked bars of runs per day with a legend.
func (d *doc) barChart(bars []bar, legend [3]string) {
	const h, axisW = 130.0, 26.0
	d.room(h + 40)
	top := d.y
	max := 0
	for _, b := range bars {
		if t := b.ok + b.partial + b.fail; t > max {
			max = t
		}
	}
	step := niceStep(max)
	ceil := step * int(math.Ceil(float64(max)/float64(step)))
	if ceil == 0 {
		ceil = step
	}
	plotX, plotW := margin+axisW, contentW-axisW
	for v := 0; v <= ceil; v += step {
		y := top + h - h*float64(v)/float64(ceil)
		d.hline(plotX, plotX+plotW, y, colLine, 0.5)
		d.textRight(plotX-6, y-4.5, fmt.Sprint(v), false, 7, colMuted)
	}
	slot := plotW / float64(len(bars))
	bw := math.Min(slot*0.62, 34)
	every := int(math.Ceil(float64(len(bars)) / 16)) // at most ~16 labels
	for i, b := range bars {
		x := plotX + float64(i)*slot + (slot-bw)/2
		y := top + h
		for _, seg := range []struct {
			n int
			c rgb
		}{{b.ok, colOK}, {b.partial, colWarn}, {b.fail, colFail}} {
			if seg.n == 0 {
				continue
			}
			sh := h * float64(seg.n) / float64(ceil)
			y -= sh
			d.rect(x, y, bw, sh, seg.c, 0)
		}
		if i%every == 0 {
			d.font(false, 7)
			d.text(x+bw/2-d.width(b.label)/2, top+h+5, b.label, false, 7, colMuted)
		}
	}
	// Legend.
	lx := plotX
	ly := top + h + 20
	for i, c := range []rgb{colOK, colWarn, colFail} {
		d.rect(lx, ly+1.5, 8, 8, c, 2)
		d.text(lx+12, ly, legend[i], false, 8, colText)
		d.font(false, 8)
		lx += 12 + d.width(legend[i]) + 18
	}
	d.y = ly + 26
}

// niceStep is a round gridline step for values up to max.
func niceStep(max int) int {
	for _, s := range []int{1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000} {
		if max/s <= 5 {
			return s
		}
	}
	return int(math.Pow(10, math.Ceil(math.Log10(float64(max)/5))))
}

// finish draws the footer on every page and returns the PDF.
func (d *doc) finish() ([]byte, error) {
	c := d.in.Catalog
	n := d.pdf.GetNumberOfPages()
	left := c.Tf("Generated by HitStor {version} on {time}", "version", d.in.Version, "time", d.in.formatTime(d.in.Generated))
	for p := 1; p <= n; p++ {
		d.check(d.pdf.SetPage(p))
		d.hline(margin, pageW-margin, footerY-6, colLine, 0.6)
		d.text(margin, footerY, left, false, 7.5, colMuted)
		d.textRight(pageW-margin, footerY, c.Tf("Page {n} of {total}", "n", p, "total", n), false, 7.5, colMuted)
	}
	if d.err != nil {
		return nil, d.err
	}
	return d.pdf.GetBytesPdfReturnErr()
}
