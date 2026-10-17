package main

// Decoding pictures, and the small grey grids they are compared as.

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
)

func decodePicture(b []byte) (image.Image, error) {
	switch {
	case bytes.HasPrefix(b, pngMagic):
		return png.Decode(bytes.NewReader(b))
	case bytes.HasPrefix(b, []byte{0xff, 0xd8}):
		return jpeg.Decode(bytes.NewReader(b))
	case bytes.HasPrefix(b, []byte("GIF8")):
		return gif.Decode(bytes.NewReader(b))
	}
	return nil, image.ErrFormat
}

func pictureSize(b []byte) (int, int, bool) {
	var cfg image.Config
	var err error
	switch {
	case bytes.HasPrefix(b, pngMagic):
		cfg, err = png.DecodeConfig(bytes.NewReader(b))
	case bytes.HasPrefix(b, []byte{0xff, 0xd8}):
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(b))
	default:
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, err == nil
}

// grid is a picture in grey, from 0 to 255.
type grid struct {
	w, h int
	v    []float32
}

func (g grid) at(x, y int) float32 { return g.v[y*g.w+x] }

// greyOf shrinks a picture to fit inside side by averaging the pixels
// that fall in each cell. Averaging rather than picking keeps a thin line
// of text in the small copy, as a thumbnail made by a camera keeps it.
func greyOf(img image.Image, side int) grid {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return grid{}
	}
	scale := math.Min(1, float64(side)/float64(max(w, h)))
	gw, gh := max(1, int(math.Round(float64(w)*scale))), max(1, int(math.Round(float64(h)*scale)))
	sum := make([]float64, gw*gh)
	cnt := make([]float64, gw*gh)
	luma := lumaOf(img)
	for y := 0; y < h; y++ {
		gy := min(gh-1, y*gh/h)
		for x := 0; x < w; x++ {
			gx := min(gw-1, x*gw/w)
			sum[gy*gw+gx] += luma(b.Min.X+x, b.Min.Y+y)
			cnt[gy*gw+gx]++
		}
	}
	g := grid{w: gw, h: gh, v: make([]float32, gw*gh)}
	for i := range g.v {
		if cnt[i] > 0 {
			g.v[i] = float32(sum[i] / cnt[i])
		}
	}
	return g
}

// lumaOf picks a fast way to read grey from the common kinds of picture,
// because At on a picture of twelve million pixels is most of a second.
func lumaOf(img image.Image) func(x, y int) float64 {
	switch p := img.(type) {
	case *image.YCbCr:
		return func(x, y int) float64 { return float64(p.Y[p.YOffset(x, y)]) }
	case *image.Gray:
		return func(x, y int) float64 { return float64(p.Pix[p.PixOffset(x, y)]) }
	case *image.NRGBA:
		return func(x, y int) float64 {
			i := p.PixOffset(x, y)
			return 0.299*float64(p.Pix[i]) + 0.587*float64(p.Pix[i+1]) + 0.114*float64(p.Pix[i+2])
		}
	case *image.RGBA:
		return func(x, y int) float64 {
			i := p.PixOffset(x, y)
			return 0.299*float64(p.Pix[i]) + 0.587*float64(p.Pix[i+1]) + 0.114*float64(p.Pix[i+2])
		}
	}
	return func(x, y int) float64 {
		r, g, b, _ := img.At(x, y).RGBA()
		return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257
	}
}

// resized stretches a grid to w by h, bilinearly.
func (g grid) resized(w, h int) grid {
	out := grid{w: w, h: h, v: make([]float32, w*h)}
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(g.h)/float64(h) - 0.5
		y0 := int(math.Floor(fy))
		ty := float32(fy - float64(y0))
		y0c, y1c := clampInt(y0, 0, g.h-1), clampInt(y0+1, 0, g.h-1)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(g.w)/float64(w) - 0.5
			x0 := int(math.Floor(fx))
			tx := float32(fx - float64(x0))
			x0c, x1c := clampInt(x0, 0, g.w-1), clampInt(x0+1, 0, g.w-1)
			top := g.at(x0c, y0c)*(1-tx) + g.at(x1c, y0c)*tx
			bottom := g.at(x0c, y1c)*(1-tx) + g.at(x1c, y1c)*tx
			out.v[y*w+x] = top*(1-ty) + bottom*ty
		}
	}
	return out
}

func (g grid) turned() grid {
	out := grid{w: g.h, h: g.w, v: make([]float32, len(g.v))}
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			out.v[x*out.w+(g.h-1-y)] = g.at(x, y)
		}
	}
	return out
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(hi, v))
}
