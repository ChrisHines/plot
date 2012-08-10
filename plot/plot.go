// Copyright 2012 The Plotinum Authors. All rights reserved.
// Use of this source code is governed by an MIT-style license
// that can be found in the LICENSE file.

// plot provides an API for setting up plots, and primitives for
// drawing on plots.
//
// Plot is the basic type for creating a plot, setting the title, axis
// labels, legend, tick marks, etc.  Types implementing the Plotter
// interface can draw to the data area of a plot using the primitives
// made available by this package.  Some standard implementations
// of the Plotter interface can be found in the
// code.google.com/p/plotinum/plotter package
// which is documented here: 
// http://go.pkgdoc.org/code.google.com/p/plotinum/plotter
package plot

import (
	"code.google.com/p/plotinum/vg"
	"code.google.com/p/plotinum/vg/veceps"
	"code.google.com/p/plotinum/vg/vecimg"
	"code.google.com/p/plotinum/vg/vecpdf"
	"code.google.com/p/plotinum/vg/vecsvg"
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"strings"
)

const (
	defaultFont = "Times-Roman"
)

// Plot is the basic type representing a plot.
type Plot struct {
	Title struct {
		// Text is the text of the plot title.  If
		// Text is the empty string then the plot
		// will not have a title.
		Text string
		TextStyle
	}

	// BackgroundColor is the background color of the plot.
	// The default is White.
	BackgroundColor color.Color

	// X and Y are the horizontal and vertical axes
	// of the plot respectively.
	X, Y Axis

	// Legend is the plot's legend.
	Legend Legend

	// plotters are drawn by calling their Plot method
	// after the axes are drawn.
	plotters []Plotter
}

// Plotter is an interface that wraps the Plot method.
// Some standard implementations of Plotter can be
// found in the code.google.com/p/plotinum/plotter
// package, documented here:
// http://go.pkgdoc.org/code.google.com/p/plotinum/plotter
type Plotter interface {
	// Plot draws the data to a DrawArea.
	Plot(DrawArea, *Plot)
}

// DataRanger wraps the DataRange method.
type DataRanger interface {
	// DataRange returns the range of X and Y values.
	DataRange() (xmin, xmax, ymin, ymax float64)
}

// New returns a new plot with some reasonable
// default settings.
func New() (*Plot, error) {
	titleFont, err := vg.MakeFont(defaultFont, 12)
	if err != nil {
		return nil, err
	}
	x, err := makeAxis()
	if err != nil {
		return nil, err
	}
	y, err := makeAxis()
	if err != nil {
		return nil, err
	}
	legend, err := makeLegend()
	if err != nil {
		return nil, err
	}
	p := &Plot{
		BackgroundColor: color.White,
		X:               x,
		Y:               y,
		Legend:          legend,
	}
	p.Title.TextStyle = TextStyle{
		Color: color.Black,
		Font:  titleFont,
	}
	return p, nil
}

// Add adds a Plotters to the plot.  If the plotters
// implements DataRanger then the minimum
// and maximum values of the X and Y axes are
// changed if necessary to fit the range of
// the data.
func (p *Plot) Add(ps ...Plotter) {
	for _, d := range ps {
		if x, ok := d.(DataRanger); ok {
			xmin, xmax, ymin, ymax := x.DataRange()
			p.X.Min = math.Min(p.X.Min, xmin)
			p.X.Max = math.Max(p.X.Max, xmax)
			p.Y.Min = math.Min(p.Y.Min, ymin)
			p.Y.Max = math.Max(p.Y.Max, ymax)
		}
	}

	p.plotters = append(p.plotters, ps...)
}

// Draw draws a plot to a DrawArea.
func (p *Plot) Draw(da *DrawArea) {
	if p.BackgroundColor != nil {
		da.SetColor(p.BackgroundColor)
		da.Fill(rectPath(da.Rect))
	}
	if p.Title.Text != "" {
		da.FillText(p.Title.TextStyle, da.Center().X, da.Max().Y, -0.5, -1, p.Title.Text)
		da.Size.Y -= p.Title.Height(p.Title.Text) - p.Title.Font.Extents().Descent
	}

	p.X.sanitizeRange()
	x := horizontalAxis{p.X}
	p.Y.sanitizeRange()
	y := verticalAxis{p.Y}

	ywidth := y.size()
	x.draw(padX(p, da.crop(ywidth, 0, 0, 0)))
	xheight := x.size()
	y.draw(padY(p, da.crop(0, xheight, 0, 0)))

	da = padY(p, padX(p, da.crop(ywidth, xheight, 0, 0)))
	for _, data := range p.plotters {
		data.Plot(*da, p)
	}
	p.Legend.draw(da)
}

// DrawGlyphBoxes draws red outlines around the plot's
// GlyphBoxes.  This is intended for debugging.
func (p *Plot) DrawGlyphBoxes(da *DrawArea) {
	da.SetColor(color.RGBA{R: 255, A: 255})
	for _, b := range p.GlyphBoxes(p) {
		b.Rect.Min.X += da.X(b.X)
		b.Rect.Min.Y += da.Y(b.Y)
		da.Stroke(rectPath(b.Rect))
	}
}

// padX returns a new DrawArea that is padded horizontally
// so that glyphs will no be clipped.
func padX(p *Plot, da *DrawArea) *DrawArea {
	glyphs := p.GlyphBoxes(p)
	l := leftMost(da, glyphs)
	xAxis := horizontalAxis{p.X}
	glyphs = append(glyphs, xAxis.GlyphBoxes(p)...)
	r := rightMost(da, glyphs)

	minx := da.Min.X - l.Min.X
	maxx := da.Max().X - (r.Min.X + r.Size.X)
	lx := vg.Length(l.X)
	rx := vg.Length(r.X)
	n := (lx*maxx - rx*minx) / (lx - rx)
	m := ((lx-1)*maxx - rx*minx + minx) / (lx - rx)
	return &DrawArea{
		vg.Canvas: vg.Canvas(da),
		Rect: Rect{
			Min:  Point{X: n, Y: da.Min.Y},
			Size: Point{X: m - n, Y: da.Size.Y},
		},
	}
}

// rightMost returns the right-most GlyphBox.
func rightMost(da *DrawArea, boxes []GlyphBox) GlyphBox {
	maxx := da.Max().X
	r := GlyphBox{X: 1}
	for _, b := range boxes {
		if x := da.X(b.X) + b.Min.X + b.Size.X; x > maxx && b.X <= 1 {
			maxx = x
			r = b
		}
	}
	return r
}

// leftMost returns the left-most GlyphBox.
func leftMost(da *DrawArea, boxes []GlyphBox) GlyphBox {
	minx := da.Min.X
	l := GlyphBox{}
	for _, b := range boxes {
		if x := da.X(b.X) + b.Min.X; x < minx && b.X >= 0 {
			minx = x
			l = b
		}
	}
	return l
}

// padY returns a new DrawArea that is padded vertically
// so that glyphs will no be clipped.
func padY(p *Plot, da *DrawArea) *DrawArea {
	glyphs := p.GlyphBoxes(p)
	b := bottomMost(da, glyphs)
	yAxis := verticalAxis{p.Y}
	glyphs = append(glyphs, yAxis.GlyphBoxes(p)...)
	t := topMost(da, glyphs)

	miny := da.Min.Y - b.Min.Y
	maxy := da.Max().Y - (t.Min.Y + t.Size.Y)
	by := vg.Length(b.Y)
	ty := vg.Length(t.Y)
	n := (by*maxy - ty*miny) / (by - ty)
	m := ((by-1)*maxy - ty*miny + miny) / (by - ty)
	return &DrawArea{
		vg.Canvas: vg.Canvas(da),
		Rect: Rect{
			Min:  Point{Y: n, X: da.Min.X},
			Size: Point{Y: m - n, X: da.Size.X},
		},
	}
}

// topMost returns the top-most GlyphBox.
func topMost(da *DrawArea, boxes []GlyphBox) GlyphBox {
	maxy := da.Max().Y
	t := GlyphBox{Y: 1}
	for _, b := range boxes {
		if y := da.Y(b.Y) + b.Min.Y + b.Size.Y; y > maxy && b.Y <= 1 {
			maxy = y
			t = b
		}
	}
	return t
}

// bottomMost returns the bottom-most GlyphBox.
func bottomMost(da *DrawArea, boxes []GlyphBox) GlyphBox {
	miny := da.Min.Y
	l := GlyphBox{}
	for _, b := range boxes {
		if y := da.Y(b.Y) + b.Min.Y; y < miny && b.Y >= 0 {
			miny = y
			l = b
		}
	}
	return l
}

// Transforms returns functions to transfrom
// from the x and y data coordinate system to
// the draw coordinate system of the given
// draw area.
func (p *Plot) Transforms(da *DrawArea) (x, y func(float64) vg.Length) {
	x = func(x float64) vg.Length { return da.X(p.X.Norm(x)) }
	y = func(y float64) vg.Length { return da.Y(p.Y.Norm(y)) }
	return
}

// GlyphBoxer wraps the GlyphBoxes method.
// It should be implemented by things that meet
// the Plotter interface that draw glyphs so that
// their glyphs are not clipped if drawn near the
// edge of the DrawArea.
type GlyphBoxer interface {
	GlyphBoxes(*Plot) []GlyphBox
}

// A GlyphBox describes the location of a glyph
// and the offset/size of its bounding box.
type GlyphBox struct {
	// The glyph location in normalized coordinates.
	X, Y float64

	// Rect is the offset of the glyph's minimum drawing
	// point relative to the glyph location and its size.
	Rect
}

// GlyphBoxes returns the GlyphBoxes for all plot
// data that meet the GlyphBoxer interface.
func (p *Plot) GlyphBoxes(*Plot) (boxes []GlyphBox) {
	for _, d := range p.plotters {
		if gb, ok := d.(GlyphBoxer); ok {
			boxes = append(boxes, gb.GlyphBoxes(p)...)
		}
	}
	return
}

// NominalX configures the plot to have a nominal X
// axis—an X axis with names instead of numbers.  The
// X location corresponding to each name are the integers,
// e.g., the x value 0 is centered above the first name and
// 1 is above the second name, etc.
func (p *Plot) NominalX(names ...string) {
	p.X.Tick.Width = 0
	p.X.Tick.Length = 0
	p.X.Width = 0
	p.Y.Padding = p.X.Tick.Label.Width(names[0]) / 2
	ticks := make([]Tick, len(names))
	for i, name := range names {
		ticks[i] = Tick{float64(i), name}
	}
	p.X.Tick.Marker = ConstantTicks(ticks)
}

// NominalY configures the plot to have a nominal Y
// axis—an Y axis with names instead of numbers.  The
// Y location corresponding to each name are the integers,
// e.g., the y value 0 is centered above the first name and
// 1 is above the second name, etc.
func (p *Plot) NominalY(names ...string) {
	p.Y.Tick.Width = 0
	p.Y.Tick.Length = 0
	p.Y.Width = 0
	p.X.Padding = p.Y.Tick.Label.Height(names[0]) / 2
	ticks := make([]Tick, len(names))
	for i, name := range names {
		ticks[i] = Tick{float64(i), name}
	}
	p.Y.Tick.Marker = ConstantTicks(ticks)
}

// Save saves the plot to an image file.  Width and height
// are specified in inches, and the file format is determined
// by the extension.  Supported extensions are
// .png, .jpg, .jpeg, .eps, .pdf, and .svg.
func (p *Plot) Save(width, height float64, file string) (err error) {
	w, h := vg.Inches(width), vg.Inches(height)
	var c vg.Canvas
	switch ext := strings.ToLower(filepath.Ext(file)); ext {
	case ".eps":
		c = veceps.New(w, h, file)
		defer c.(*veceps.Canvas).Save(file)
	case ".png":
		c, err = vecimg.New(w, h)
		if err != nil {
			return
		}
		defer func() { err = c.(*vecimg.Canvas).SavePNG(file) }()
	case ".jpg", ".jpeg":
		c, err = vecimg.New(w, h)
		if err != nil {
			return
		}
		defer func() { err = c.(*vecimg.Canvas).SaveJPEG(file) }()
	case ".svg":
		c = vecsvg.New(w, h)
		defer func() { err = c.(*vecsvg.Canvas).Save(file) }()
	case ".pdf":
		c = vecpdf.New(w, h)
		defer func() { err = c.(*vecpdf.Canvas).Save(file) }()
	default:
		return fmt.Errorf("Unsupported file extension: %s", ext)
	}
	p.Draw(NewDrawArea(c, w, h))
	return
}
