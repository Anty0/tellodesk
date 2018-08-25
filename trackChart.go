package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/texture"
)

type trackChartT struct {
	gui.Panel
	track                           *telloTrack
	tex                             *texture.Texture2D
	backingImage                    *image.RGBA
	width, height, xOrigin, yOrigin int
	bgCol, axesCol, labelCol        color.Color
	maxOffset                       float32
	scalePPM                        float32 // scale factor expressed as Pixels Per Metre
}

const defaultTrackScale float32 = 10.0

func buildTrackChart(w, h int, scale float32) (tc *trackChartT) {
	tc = new(trackChartT)
	tc.width = w
	tc.height = h
	tc.Panel.Initialize(float32(tc.width), float32(tc.height))
	tc.xOrigin = w / 2
	tc.yOrigin = h / 2
	tc.bgCol = color.White
	tc.axesCol = color.RGBA{128, 128, 128, 255} // color.Black
	tc.labelCol = color.RGBA{128, 128, 128, 255}
	tc.maxOffset = scale
	tc.scalePPM = float32(tc.yOrigin) / scale // TODO - we assume here that height <= width
	tc.backingImage = image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{w, h}})
	tc.tex = texture.NewTexture2DFromRGBA(tc.backingImage)
	tc.tex.SetMagFilter(gls.NEAREST)
	tc.tex.SetMinFilter(gls.NEAREST)
	tc.Panel.Material().AddTexture(tc.tex)
	tc.track = newTrack()
	tc.drawEmptyChart()
	return tc
}

func (tc *trackChartT) clearChart() {
	draw.Draw(tc.backingImage, tc.backingImage.Bounds(), image.NewUniform(tc.bgCol), image.ZP, draw.Src)
	tc.tex.SetFromRGBA(tc.backingImage)
}

func (tc *trackChartT) drawEmptyChart() {
	tc.clearChart()
	// blank vertical axis
	for y := 0; y < tc.height; y++ {
		tc.backingImage.Set(tc.xOrigin, y, tc.axesCol)
	}
	// blank horizontal axis
	for x := 0; x < tc.width; x++ {
		tc.backingImage.Set(x, tc.yOrigin, tc.axesCol)
	}
	// x-axis labels
	for x := -tc.maxOffset; x <= tc.maxOffset; x++ {
		tc.backingImage.Set(tc.xOrigin+int(x*tc.scalePPM), tc.yOrigin-1, tc.axesCol)
		tc.backingImage.Set(tc.xOrigin+int(x*tc.scalePPM), tc.yOrigin+1, tc.axesCol)
		tc.drawLabel(x, 0, strconv.Itoa(int(x)))
	}
	// y-axis labels
	for y := -tc.maxOffset; y <= tc.maxOffset; y++ {
		tc.backingImage.Set(tc.xOrigin-1, tc.yOrigin+int(y*tc.scalePPM), tc.axesCol)
		tc.backingImage.Set(tc.xOrigin+1, tc.yOrigin+int(y*tc.scalePPM), tc.axesCol)
		tc.drawLabel(0, y, strconv.Itoa(int(y)))
		fmt.Printf("Y label drawn at: %f\n", y)
	}
	tc.tex.SetFromRGBA(tc.backingImage)
}

func (tc *trackChartT) drawLabel(x, y float32, lab string) {
	point := fixed.Point26_6{
		X: fixed.Int26_6(tc.xToOrd(x) * 64),
		Y: fixed.Int26_6(tc.yToOrd(y) * 64)}
	d := &font.Drawer{
		Dst:  tc.backingImage,
		Src:  image.NewUniform(tc.labelCol),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(lab)
}

func (tc *trackChartT) xToOrd(x float32) (xOrd int) {
	xOrd = (int(float32(tc.xOrigin) + x*tc.scalePPM))
	return xOrd
}

func (tc *trackChartT) yToOrd(y float32) (yOrd int) {
	yOrd = tc.height - (int(float32(tc.yOrigin) + y*tc.scalePPM))
	return yOrd
}
