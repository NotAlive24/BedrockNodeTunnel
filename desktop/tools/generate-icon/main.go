package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
)

func main() {
	const size = 512
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 9, G: 13, B: 22, A: 255}}, image.Point{}, draw.Src)

	// Rounded-square background.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-size/2, y-size/2
			if dx*dx+dy*dy > 224*224 && (x < 112 || x > 400) && (y < 112 || y > 400) {
				img.SetRGBA(x, y, color.RGBA{})
			}
		}
	}

	// A simple, crisp N mark with a teal-to-blue diagonal accent.
	accent := color.RGBA{R: 74, G: 211, B: 220, A: 255}
	drawRect(img, 112, 166, 174, 346, accent)
	drawRect(img, 338, 166, 400, 346, color.RGBA{R: 56, G: 174, B: 239, A: 255})
	for i := 0; i < 164; i++ {
		x := 174 + i
		y := 166 + i
		for t := -30; t <= 30; t++ {
			px, py := x+t, y
			if image.Pt(px, py).In(img.Bounds()) {
				img.SetRGBA(px, py, color.RGBA{R: uint8(74 + (i* -18 / 164)), G: uint8(211 - (i*37/164)), B: 220, A: 255})
			}
		}
	}
	f, err := os.Create("build/appicon.png")
	if err != nil { panic(err) }
	defer f.Close()
	if err := png.Encode(f, img); err != nil { panic(err) }
}

func drawRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{C:c}, image.Point{}, draw.Src)
}
