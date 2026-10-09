//go:build ignore

// gen renders the extension icons: go run icons/gen.go (from extension/).
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strconv"
)

const ss = 4 // supersampling factor

func distSeg(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func inRoundRect(x, y, r float64) bool {
	cx := math.Max(r, math.Min(1-r, x))
	cy := math.Max(r, math.Min(1-r, y))
	return math.Hypot(x-cx, y-cy) <= r
}

func render(size int) *image.NRGBA {
	n := size * ss
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	stroke := 0.095
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px*ss+sx) + 0.5) / float64(n)
					y := (float64(py*ss+sy) + 0.5) / float64(n)
					if !inRoundRect(x, y, 0.23) {
						continue
					}
					t := (x + y) / 2
					cr := 108 + (155-108)*t
					cg := 147 + (107-147)*t
					cb := 255.0
					arrow := distSeg(x, y, 0.5, 0.22, 0.5, 0.57) <= stroke/2 ||
						distSeg(x, y, 0.33, 0.42, 0.5, 0.59) <= stroke/2 ||
						distSeg(x, y, 0.67, 0.42, 0.5, 0.59) <= stroke/2 ||
						distSeg(x, y, 0.29, 0.75, 0.71, 0.75) <= stroke/2
					if arrow {
						cr, cg, cb = 255, 255, 255
					}
					r, g, b, a = r+cr, g+cg, b+cb, a+1
				}
			}
			if a == 0 {
				continue
			}
			total := float64(ss * ss)
			img.SetNRGBA(px, py, color.NRGBA{
				R: uint8(r / a), G: uint8(g / a), B: uint8(b / a), A: uint8(255 * a / total),
			})
		}
	}
	return img
}

func main() {
	for _, size := range []int{16, 32, 48, 128} {
		f, err := os.Create("icons/icon" + strconv.Itoa(size) + ".png")
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, render(size)); err != nil {
			panic(err)
		}
		f.Close()
	}
}
