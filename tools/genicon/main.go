package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	langEnglish = 0x0409
)

type iconImage struct {
	size int
	png  []byte
}

func main() {
	must(os.MkdirAll("assets", 0755))

	images := make([]iconImage, 0, 6)
	for _, size := range []int{16, 32, 48, 64, 128, 256} {
		images = append(images, iconImage{size: size, png: renderIcon(size)})
	}

	must(os.WriteFile(filepath.Join("assets", "app.ico"), buildICO(images), 0644))
	must(os.WriteFile("resource.syso", buildResourceObject(images), 0644))
}

func renderIcon(size int) []byte {
	scale := 4
	canvas := image.NewRGBA(image.Rect(0, 0, size*scale, size*scale))
	w := canvas.Bounds().Dx()
	h := canvas.Bounds().Dy()

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !insideRoundRect(float64(x), float64(y), 0, 0, float64(w), float64(h), float64(w)*0.22) {
				continue
			}
			tx := float64(x) / float64(w)
			ty := float64(y) / float64(h)
			r := uint8(16 + tx*18)
			g := uint8(112 + ty*52)
			b := uint8(151 + tx*30)
			canvas.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	drawCircle(canvas, 0.72, 0.25, 0.13, color.RGBA{R: 77, G: 211, B: 167, A: 255})
	drawSheet(canvas)
	drawPlay(canvas)
	drawWave(canvas)

	return encodePNG(downsample(canvas, size))
}

func drawSheet(img *image.RGBA) {
	w := float64(img.Bounds().Dx())
	h := float64(img.Bounds().Dy())
	x0, y0 := w*0.23, h*0.19
	x1, y1 := w*0.71, h*0.78
	r := w * 0.06
	for y := int(y0); y < int(y1); y++ {
		for x := int(x0); x < int(x1); x++ {
			if !insideRoundRect(float64(x), float64(y), x0, y0, x1-x0, y1-y0, r) {
				continue
			}
			img.SetRGBA(x, y, color.RGBA{R: 247, G: 252, B: 255, A: 238})
		}
	}

	for _, yy := range []float64{0.48, 0.58, 0.68} {
		drawRoundRect(img, 0.40, yy, 0.61, yy+0.035, 0.012, color.RGBA{R: 37, G: 94, B: 121, A: 220})
	}
}

func drawPlay(img *image.RGBA) {
	w := float64(img.Bounds().Dx())
	h := float64(img.Bounds().Dy())
	p1 := point{w * 0.36, h * 0.30}
	p2 := point{w * 0.36, h * 0.45}
	p3 := point{w * 0.50, h * 0.375}
	for y := int(p1.y); y <= int(p2.y); y++ {
		for x := int(p1.x); x <= int(p3.x); x++ {
			if pointInTriangle(point{float64(x), float64(y)}, p1, p2, p3) {
				img.SetRGBA(x, y, color.RGBA{R: 6, G: 129, B: 225, A: 255})
			}
		}
	}
}

func drawWave(img *image.RGBA) {
	for i, height := range []float64{0.10, 0.18, 0.28, 0.18, 0.10} {
		x := 0.30 + float64(i)*0.047
		drawRoundRect(img, x, 0.78-height, x+0.024, 0.78, 0.012, color.RGBA{R: 91, G: 226, B: 181, A: 255})
	}
}

func drawCircle(img *image.RGBA, cx, cy, radius float64, c color.RGBA) {
	w := float64(img.Bounds().Dx())
	h := float64(img.Bounds().Dy())
	r := radius * w
	px := cx * w
	py := cy * h
	for y := int(py - r); y <= int(py+r); y++ {
		for x := int(px - r); x <= int(px+r); x++ {
			if x < 0 || y < 0 || x >= int(w) || y >= int(h) {
				continue
			}
			if math.Hypot(float64(x)-px, float64(y)-py) <= r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func drawRoundRect(img *image.RGBA, x0, y0, x1, y1, radius float64, c color.RGBA) {
	w := float64(img.Bounds().Dx())
	h := float64(img.Bounds().Dy())
	left, top := x0*w, y0*h
	right, bottom := x1*w, y1*h
	r := radius * w
	for y := int(top); y < int(bottom); y++ {
		for x := int(left); x < int(right); x++ {
			if insideRoundRect(float64(x), float64(y), left, top, right-left, bottom-top, r) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func insideRoundRect(x, y, left, top, width, height, radius float64) bool {
	right := left + width
	bottom := top + height
	if x < left || x >= right || y < top || y >= bottom {
		return false
	}
	cx := math.Max(left+radius, math.Min(x, right-radius))
	cy := math.Max(top+radius, math.Min(y, bottom-radius))
	return math.Hypot(x-cx, y-cy) <= radius
}

type point struct {
	x float64
	y float64
}

func pointInTriangle(p, a, b, c point) bool {
	d1 := sign(p, a, b)
	d2 := sign(p, b, c)
	d3 := sign(p, c, a)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func sign(p1, p2, p3 point) float64 {
	return (p1.x-p3.x)*(p2.y-p3.y) - (p2.x-p3.x)*(p1.y-p3.y)
}

func downsample(src *image.RGBA, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	ratio := src.Bounds().Dx() / size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for yy := 0; yy < ratio; yy++ {
				for xx := 0; xx < ratio; xx++ {
					cr, cg, cb, ca := src.At(x*ratio+xx, y*ratio+yy).RGBA()
					r += cr
					g += cg
					b += cb
					a += ca
				}
			}
			div := uint32(ratio * ratio)
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8((r / div) >> 8),
				G: uint8((g / div) >> 8),
				B: uint8((b / div) >> 8),
				A: uint8((a / div) >> 8),
			})
		}
	}
	return dst
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	must(png.Encode(&buf, img))
	return buf.Bytes()
}

func buildICO(images []iconImage) []byte {
	var buf bytes.Buffer
	write16(&buf, 0)
	write16(&buf, 1)
	write16(&buf, uint16(len(images)))
	offset := 6 + len(images)*16
	for _, img := range images {
		writeIconDirEntry(&buf, img.size, len(img.png), offset, false)
		offset += len(img.png)
	}
	for _, img := range images {
		buf.Write(img.png)
	}
	return buf.Bytes()
}

func buildGroupIcon(images []iconImage) []byte {
	var buf bytes.Buffer
	write16(&buf, 0)
	write16(&buf, 1)
	write16(&buf, uint16(len(images)))
	for i, img := range images {
		writeIconDirEntry(&buf, img.size, len(img.png), i+1, true)
	}
	return buf.Bytes()
}

func writeIconDirEntry(buf *bytes.Buffer, size, bytesInRes, last int, group bool) {
	if size >= 256 {
		buf.WriteByte(0)
	} else {
		buf.WriteByte(byte(size))
	}
	if size >= 256 {
		buf.WriteByte(0)
	} else {
		buf.WriteByte(byte(size))
	}
	buf.WriteByte(0)
	buf.WriteByte(0)
	write16(buf, 1)
	write16(buf, 32)
	write32(buf, uint32(bytesInRes))
	if group {
		write16(buf, uint16(last))
	} else {
		write32(buf, uint32(last))
	}
}

func buildResourceObject(images []iconImage) []byte {
	section := buildResourceSection(images)
	var buf bytes.Buffer
	write16(&buf, 0x8664)
	write16(&buf, 1)
	write32(&buf, uint32(time.Now().Unix()))
	write32(&buf, uint32(20+40+len(section.data)+len(section.relocations)*10))
	write32(&buf, 2)
	write16(&buf, 0)
	write16(&buf, 0)

	var name [8]byte
	copy(name[:], ".rsrc")
	buf.Write(name[:])
	write32(&buf, 0)
	write32(&buf, 0)
	write32(&buf, uint32(len(section.data)))
	write32(&buf, 20+40)
	write32(&buf, uint32(20+40+len(section.data)))
	write32(&buf, 0)
	write16(&buf, uint16(len(section.relocations)))
	write16(&buf, 0)
	write32(&buf, 0x40000040)

	buf.Write(section.data)
	for _, offset := range section.relocations {
		write32(&buf, uint32(offset))
		write32(&buf, 0)
		write16(&buf, 0x0003)
	}

	writeSymbol(&buf, ".rsrc", 0, 1, 0, 3, 1)
	write32(&buf, uint32(len(section.data)))
	write16(&buf, uint16(len(section.relocations)))
	write16(&buf, 0)
	write32(&buf, 0)
	write16(&buf, 1)
	buf.Write([]byte{0, 0, 0, 0})
	write32(&buf, 4)
	return buf.Bytes()
}

type resourceSection struct {
	data        []byte
	relocations []int
}

type resourceData struct {
	typ  uint32
	id   uint32
	data []byte
}

func buildResourceSection(images []iconImage) resourceSection {
	group := buildGroupIcon(images)
	items := make([]resourceData, 0, len(images)+1)
	for i, img := range images {
		items = append(items, resourceData{typ: rtIcon, id: uint32(i + 1), data: img.png})
	}
	items = append(items, resourceData{typ: rtGroupIcon, id: 1, data: group})

	var dataEntries bytes.Buffer
	var payload bytes.Buffer

	typeDirCount := 2
	rootOff := 0
	iconTypeOff := 16 + typeDirCount*8
	groupTypeOff := iconTypeOff + 16 + len(images)*8
	iconLangOff := groupTypeOff + 16 + 8
	groupLangOff := iconLangOff + len(images)*(16+8)
	dataEntryBase := groupLangOff + 16 + 8

	payloadBase := align4(dataEntryBase + len(items)*16)
	currentPayloadOff := payloadBase
	dataEntryOffsets := make([]int, len(items))
	payloadOffsets := make([]int, len(items))
	relocations := make([]int, len(items))
	for i, item := range items {
		dataEntryOffsets[i] = dataEntryBase + i*16
		relocations[i] = dataEntryOffsets[i]
		payloadOffsets[i] = currentPayloadOff
		currentPayloadOff = align4(currentPayloadOff + len(item.data))
	}

	var buf bytes.Buffer
	writeResourceDir(&buf, 0, 0, 0, uint16(typeDirCount))
	writeDirEntry(&buf, rtIcon, iconTypeOff, true)
	writeDirEntry(&buf, rtGroupIcon, groupTypeOff, true)

	writeResourceDir(&buf, 0, 0, 0, uint16(len(images)))
	for i := range images {
		writeDirEntry(&buf, uint32(i+1), iconLangOff+i*(16+8), true)
	}

	writeResourceDir(&buf, 0, 0, 0, 1)
	writeDirEntry(&buf, 1, groupLangOff, true)

	for i := range images {
		writeResourceDir(&buf, 0, 0, 0, 1)
		writeDirEntry(&buf, langEnglish, dataEntryOffsets[i], false)
	}

	writeResourceDir(&buf, 0, 0, 0, 1)
	writeDirEntry(&buf, langEnglish, dataEntryOffsets[len(items)-1], false)

	for i, item := range items {
		write32(&dataEntries, uint32(payloadOffsets[i]))
		write32(&dataEntries, uint32(len(item.data)))
		write32(&dataEntries, 0)
		write32(&dataEntries, 0)
	}
	buf.Write(dataEntries.Bytes())

	for buf.Len() < payloadBase {
		buf.WriteByte(0)
	}
	for i, item := range items {
		for buf.Len() < payloadOffsets[i] {
			buf.WriteByte(0)
		}
		payload.Write(item.data)
		buf.Write(payload.Bytes())
		payload.Reset()
	}

	if buf.Len() <= rootOff {
		panic("resource section is empty")
	}
	return resourceSection{data: buf.Bytes(), relocations: relocations}
}

func writeResourceDir(buf *bytes.Buffer, characteristics, timestamp uint32, major, entries uint16) {
	write32(buf, characteristics)
	write32(buf, timestamp)
	write16(buf, major)
	write16(buf, 0)
	write16(buf, 0)
	write16(buf, entries)
}

func writeDirEntry(buf *bytes.Buffer, id uint32, offset int, dir bool) {
	write32(buf, id)
	value := uint32(offset)
	if dir {
		value |= 0x80000000
	}
	write32(buf, value)
}

func writeSymbol(buf *bytes.Buffer, name string, value uint32, sectionNumber int16, typ uint16, storageClass byte, auxCount byte) {
	var rawName [8]byte
	copy(rawName[:], name)
	buf.Write(rawName[:])
	write32(buf, value)
	must(binary.Write(buf, binary.LittleEndian, sectionNumber))
	write16(buf, typ)
	buf.WriteByte(storageClass)
	buf.WriteByte(auxCount)
}

func align4(v int) int {
	return (v + 3) &^ 3
}

func write16(buf *bytes.Buffer, v uint16) {
	must(binary.Write(buf, binary.LittleEndian, v))
}

func write32(buf *bytes.Buffer, v uint32) {
	must(binary.Write(buf, binary.LittleEndian, v))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
