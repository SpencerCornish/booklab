package display

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"rsc.io/qr"
)

//go:embed fonts/IBMPlexMono-Regular.ttf
var plexRegularTTF []byte

//go:embed fonts/IBMPlexMono-Bold.ttf
var plexBoldTTF []byte

type weight int

const (
	regular weight = iota
	bold
)

var (
	fontsOnce sync.Once
	fontsErr  error
	fonts     [2]*opentype.Font
)

func face(w weight, size float64) font.Face {
	fontsOnce.Do(func() {
		for i, data := range [][]byte{plexRegularTTF, plexBoldTTF} {
			f, err := opentype.Parse(data)
			if err != nil {
				fontsErr = err
				return
			}
			fonts[i] = f
		}
	})
	if fontsErr != nil {
		// The fonts are embedded; failing to parse them is a build problem.
		panic(fmt.Sprintf("display: parse embedded font: %v", fontsErr))
	}
	// Return a new face every time: *opentype.Face is not safe for concurrent
	// use (it reuses an internal sfnt.Buffer, rasterizer and mask), and renders
	// can overlap across display polls and admin previews. The shared
	// *opentype.Font is safe to use concurrently, and creating a face is cheap
	// next to encoding the PNG.
	f, err := opentype.NewFace(fonts[w], &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(fmt.Sprintf("display: new face: %v", err))
	}
	return f
}

const (
	black uint8 = 0
	white uint8 = 255
)

// canvas draws in 8-bit gray so text keeps its anti-aliased shape, then
// thresholds to 1 bit on encode. Fills and patterns only ever use pure black
// and white, so they survive the threshold untouched.
type canvas struct {
	img *image.Gray
}

func newCanvas() *canvas {
	c := &canvas{img: image.NewGray(image.Rect(0, 0, Width, Height))}
	c.fill(c.img.Bounds(), white)
	return c
}

func (c *canvas) set(x, y int, v uint8) {
	if image.Pt(x, y).In(c.img.Rect) {
		c.img.Pix[c.img.PixOffset(x, y)] = v
	}
}

func (c *canvas) fill(r image.Rectangle, v uint8) {
	r = r.Intersect(c.img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := c.img.Pix[c.img.PixOffset(r.Min.X, y):c.img.PixOffset(r.Max.X, y)]
		for i := range row {
			row[i] = v
		}
	}
}

func (c *canvas) stroke(r image.Rectangle, w int, v uint8) {
	c.fill(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), v)
	c.fill(image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), v)
	c.fill(image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), v)
	c.fill(image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y), v)
}

// dottedStroke draws a 1px outline with every other pixel set.
func (c *canvas) dottedStroke(r image.Rectangle) {
	for x := r.Min.X; x < r.Max.X; x += 2 {
		c.set(x, r.Min.Y, black)
		c.set(x, r.Max.Y-1, black)
	}
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		c.set(r.Min.X, y, black)
		c.set(r.Max.X-1, y, black)
	}
}

// dither fills r with an ordered pattern. every=2 is a 50% checkerboard;
// every=4 sets one pixel in a 2x2 block on alternate rows (~12%).
func (c *canvas) dither(r image.Rectangle, every int) {
	r = r.Intersect(c.img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			var on bool
			if every <= 2 {
				on = (x+y)%2 == 0
			} else {
				on = x%every == 0 && y%every == 0 || (x+every/2)%every == 0 && (y+every/2)%every == 0
			}
			if on {
				c.set(x, y, black)
			}
		}
	}
}

// hatch fills r with 45° lines spaced gap pixels apart.
func (c *canvas) hatch(r image.Rectangle, gap int) {
	r = r.Intersect(c.img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if (x+y)%gap == 0 {
				c.set(x, y, black)
			}
		}
	}
}

// triangleLeft draws a filled triangle pointing left with its tip at (x, cy).
func (c *canvas) triangleLeft(x, cy, w, h int) {
	for dx := 0; dx < w; dx++ {
		half := h * dx / (2 * w)
		for dy := -half; dy <= half; dy++ {
			c.set(x+dx, cy+dy, black)
		}
	}
}

func textWidth(f font.Face, s string) int {
	return font.MeasureString(f, s).Ceil()
}

func (c *canvas) text(f font.Face, x, baseline int, s string, v uint8) {
	d := &font.Drawer{
		Dst:  c.img,
		Src:  image.NewUniform(color.Gray{Y: v}),
		Face: f,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(s)
}

// truncate shortens s with an ellipsis until it fits maxW.
func truncate(f font.Face, s string, maxW int) string {
	if textWidth(f, s) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		cand := strings.TrimRight(string(r), " ") + "…"
		if textWidth(f, cand) <= maxW {
			return cand
		}
	}
	return ""
}

// fitFace returns the largest size from sizes at which s fits maxW, or the
// smallest size if none do.
func fitFace(w weight, s string, maxW int, sizes ...float64) font.Face {
	for _, sz := range sizes {
		if f := face(w, sz); textWidth(f, s) <= maxW {
			return f
		}
	}
	return face(w, sizes[len(sizes)-1])
}

// wrap breaks s into lines no wider than maxW, splitting long words (URLs) at
// any rune when they cannot fit on their own.
func wrap(f font.Face, s string, maxW int) []string {
	var lines []string
	cur := ""
	push := func() {
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
	}
	for _, word := range strings.Fields(s) {
		cand := word
		if cur != "" {
			cand = cur + " " + word
		}
		if textWidth(f, cand) <= maxW {
			cur = cand
			continue
		}
		push()
		for textWidth(f, word) > maxW {
			r := []rune(word)
			n := len(r)
			for n > 1 && textWidth(f, string(r[:n])) > maxW {
				n--
			}
			// Prefer to break a URL just after a '.', '/' or '-'.
			for k := n; k > n/2; k-- {
				if strings.ContainsRune("./-", r[k-1]) {
					n = k
					break
				}
			}
			lines = append(lines, string(r[:n]))
			word = string(r[n:])
		}
		cur = word
	}
	push()
	return lines
}

// vcenter returns the baseline that vertically centers a line of f in [top, top+h).
func vcenter(f font.Face, top, h int) int {
	m := f.Metrics()
	asc, desc := m.Ascent.Ceil(), m.Descent.Ceil()
	return top + (h-(asc+desc))/2 + asc
}

func (c *canvas) qr(url string, x, y, size int) {
	code, err := qr.Encode(url, qr.M)
	if err != nil {
		return
	}
	scale := size / code.Size
	if scale < 1 {
		return
	}
	off := (size - code.Size*scale) / 2
	for qy := 0; qy < code.Size; qy++ {
		for qx := 0; qx < code.Size; qx++ {
			if code.Black(qx, qy) {
				px, py := x+off+qx*scale, y+off+qy*scale
				c.fill(image.Rect(px, py, px+scale, py+scale), black)
			}
		}
	}
}

func (c *canvas) battery(x, y int) {
	// 26x12 body with a nub, one short bar inside: "low".
	c.stroke(image.Rect(x, y, x+26, y+12), 2, black)
	c.fill(image.Rect(x+26, y+3, x+29, y+9), black)
	c.fill(image.Rect(x+4, y+4, x+8, y+8), black)
}

func (c *canvas) png() ([]byte, error) {
	out := image.NewPaletted(c.img.Rect, color.Palette{color.Gray{Y: 0}, color.Gray{Y: 255}})
	for i, v := range c.img.Pix {
		if v >= 128 {
			out.Pix[i] = 1
		}
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Layout constants shared by the screens.
const (
	margin    = 24
	splitX    = 320 // left/right panel divider
	headerY   = 52  // header baseline
	headerRul = 68  // y of the rule under the headers
)

// RenderSchedule draws the door-sign screen.
func RenderSchedule(v View) ([]byte, error) {
	c := newCanvas()
	drawLeftPanel(c, v)
	c.fill(image.Rect(splitX-1, margin, splitX+1, Height-margin), black)
	drawTimeline(c, v)
	return c.png()
}

func drawLeftPanel(c *canvas, v View) {
	x0, w := margin, splitX-2*margin

	title := strings.ToUpper(v.Title)
	tf := fitFace(bold, title, w, 26, 22, 18)
	c.text(tf, x0, headerY, truncate(tf, title, w), black)
	c.fill(image.Rect(x0, headerRul, x0+w, headerRul+2), black)

	// Status banner.
	banner := image.Rect(x0, 88, x0+w, 176)
	textV := black
	switch v.Status {
	case StatusInUse:
		c.fill(banner, black)
		textV = white
	case StatusClosed:
		c.dither(banner, 2)
		c.fill(banner.Inset(8), white)
	default:
		c.stroke(banner, 4, black)
	}
	hf := fitFace(bold, v.Headline, banner.Dx()-32, 56, 48, 40, 32, 26)
	hw := textWidth(hf, v.Headline)
	c.text(hf, banner.Min.X+(banner.Dx()-hw)/2, vcenter(hf, banner.Min.Y, banner.Dy()), v.Headline, textV)

	y := banner.Max.Y + 38
	if v.Detail != "" {
		df := fitFace(regular, v.Detail, w, 24, 20, 18)
		c.text(df, x0, y, truncate(df, v.Detail, w), black)
		y += 34
	}
	if v.DetailStrong != "" {
		sf := fitFace(bold, v.DetailStrong, w, 26, 22, 18)
		c.text(sf, x0, y, truncate(sf, v.DetailStrong, w), black)
		y += 34
	}
	if v.Next != "" {
		y += 6
		nf := face(regular, 18)
		for i, line := range wrap(nf, v.Next, w) {
			if i == 2 {
				break
			}
			c.text(nf, x0, y, line, black)
			y += 24
		}
	}

	// QR block, anchored to the bottom.
	const qrSize = 108
	qrY := Height - margin - qrSize
	if v.QRURL != "" && y < qrY {
		c.qr(v.QRURL, x0, qrY, qrSize)
		lx := x0 + qrSize + 12
		lf := face(regular, 16)
		ly := qrY + 20
		for i, line := range wrap(lf, v.QRLabel, x0+w-lx) {
			if i == 5 {
				break
			}
			c.text(lf, lx, ly, line, black)
			ly += 20
		}
	}

	if v.BatteryLow {
		c.battery(x0+w-29, Height-margin-10)
	}
}

func drawTimeline(c *canvas, v View) {
	x0 := splitX + margin
	x1 := Width - margin
	df := fitFace(bold, v.DayLabel, x1-x0, 22, 18, 16)
	c.text(df, x0, headerY, truncate(df, v.DayLabel, x1-x0), black)
	c.fill(image.Rect(x0, headerRul, x1, headerRul+2), black)

	top, bottom := headerRul+16, Height-margin
	if len(v.Rows) == 0 {
		mf := face(bold, 26)
		lines := wrap(mf, v.EmptyMessage, x1-x0-16)
		lh := 36
		y := top + (bottom-top-len(lines)*lh)/2 + 26
		for _, l := range lines {
			c.text(mf, x0+(x1-x0-textWidth(mf, l))/2, y, l, black)
			y += lh
		}
		return
	}

	rowH := min(44, (bottom-top)/len(v.Rows))
	labelSize, barTextSize := 20.0, 20.0
	switch {
	case rowH < 26:
		labelSize, barTextSize = 13, 13
	case rowH < 34:
		labelSize, barTextSize = 16, 16
	}
	lf := face(regular, labelSize)
	lfb := face(bold, labelSize)
	const labelW = 72
	barX0 := x0 + labelW + 8
	barX1 := x1 - 26 // room for the "now" marker
	pad := max(2, rowH/10)

	for i, row := range v.Rows {
		y := top + i*rowH
		bar := image.Rect(barX0, y+pad, barX1, y+rowH-pad)

		labF := lf
		if row.Current {
			labF = lfb
		}
		c.text(labF, x0+labelW-textWidth(labF, row.Label), vcenter(labF, y, rowH), row.Label, black)

		bf := face(bold, barTextSize)
		rf := face(regular, barTextSize)
		maxTextW := bar.Dx() - 24
		switch row.State {
		case RowBooked:
			name := row.Text
			if name == "" {
				name = "Booked"
			}
			if row.Past {
				c.dither(bar, 4)
				c.dottedStroke(bar)
				s := truncate(rf, name, maxTextW)
				plateText(c, rf, bar, s)
			} else {
				c.fill(bar, black)
				s := truncate(bf, name, maxTextW)
				c.text(bf, bar.Min.X+12, vcenter(bf, bar.Min.Y, bar.Dy()), s, white)
			}
		case RowClosed:
			c.hatch(bar, 6)
			c.stroke(bar, 1, black)
			plateText(c, rf, bar, "closed")
		default:
			if row.Past {
				c.dottedStroke(bar)
			} else if row.Current {
				c.stroke(bar, 3, black)
			} else {
				c.stroke(bar, 1, black)
			}
			c.text(rf, bar.Min.X+12, vcenter(rf, bar.Min.Y, bar.Dy()), "open", black)
		}
		if row.Current {
			c.triangleLeft(barX1+6, y+rowH/2, 18, 20)
		}
	}
}

// plateText draws s at the left of bar on a white plate so it stays legible
// over a pattern.
func plateText(c *canvas, f font.Face, bar image.Rectangle, s string) {
	tw := textWidth(f, s)
	plate := image.Rect(bar.Min.X+6, bar.Min.Y+3, bar.Min.X+18+tw, bar.Max.Y-3)
	c.fill(plate, white)
	c.text(f, bar.Min.X+12, vcenter(f, bar.Min.Y, bar.Dy()), s, black)
}

// RenderNotice draws a full-screen message.
func RenderNotice(n NoticeView) ([]byte, error) {
	c := newCanvas()
	c.stroke(image.Rect(margin, margin, Width-margin, Height-margin), 4, black)

	inner := Width - 4*margin
	tf := fitFace(bold, n.Title, inner, 44, 36, 30)
	y := 120
	c.text(tf, (Width-textWidth(tf, n.Title))/2, y, n.Title, black)
	y += 52

	bf := face(regular, 24)
	for _, line := range n.Lines {
		for _, l := range wrap(bf, line, inner) {
			c.text(bf, (Width-textWidth(bf, l))/2, y, l, black)
			y += 34
		}
	}

	if n.Code != "" {
		cf := fitFace(bold, n.Code, inner-48, 72, 56, 40)
		cw := textWidth(cf, n.Code)
		box := image.Rect((Width-cw)/2-24, y+8, (Width+cw)/2+24, y+8+100)
		c.fill(box, black)
		c.text(cf, box.Min.X+24, vcenter(cf, box.Min.Y, box.Dy()), n.Code, white)
	}

	if n.Footer != "" {
		ff := face(regular, 16)
		c.text(ff, (Width-textWidth(ff, n.Footer))/2, Height-margin-20, n.Footer, black)
	}
	return c.png()
}
