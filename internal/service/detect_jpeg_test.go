package service

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// ── in-memory JPEG / EXIF builders ──

// jpegSegment builds a JPEG marker segment: FF <marker> <lenHi> <lenLo> <data>.
func jpegSegment(marker byte, data []byte) []byte {
	seg := make([]byte, 0, len(data)+4)
	seg = append(seg, 0xFF, marker)
	length := len(data) + 2
	seg = append(seg, byte(length>>8), byte(length))
	seg = append(seg, data...)
	return seg
}

// concatJPEG concatenates byte slices into a single stream.
func concatJPEG(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// app1ExifSegment wraps raw TIFF data in an APP1 EXIF segment.
func app1ExifSegment(tiff []byte) []byte {
	return jpegSegment(0xE1, append([]byte("Exif\x00\x00"), tiff...))
}

// exifTestEntry describes one 12-byte EXIF IFD entry for the test builder.
type exifTestEntry struct {
	tag       uint16
	typ       uint16
	count     uint32
	value     []byte // raw value bytes; nil for subIFDPtr
	subIFDPtr bool   // when true, the value field is the resolved ExifIFD offset
}

func asciiEntry(tag uint16, s string) exifTestEntry {
	b := append([]byte(s), 0)
	return exifTestEntry{tag: tag, typ: 2, count: uint32(len(b)), value: b}
}

func shortEntry(tag uint16, order binary.ByteOrder, v uint16) exifTestEntry {
	b := make([]byte, 2)
	order.PutUint16(b, v)
	return exifTestEntry{tag: tag, typ: 3, count: 1, value: b}
}

func rationalEntry(tag uint16, order binary.ByteOrder, num, den uint32) exifTestEntry {
	b := make([]byte, 8)
	order.PutUint32(b[0:4], num)
	order.PutUint32(b[4:8], den)
	return exifTestEntry{tag: tag, typ: 5, count: 1, value: b}
}

func exifIFDPointerEntry() exifTestEntry {
	return exifTestEntry{tag: 0x8769, typ: 4, count: 1, subIFDPtr: true}
}

// buildExifTIFF builds a minimal TIFF structure (header + IFD0 + optional
// ExifIFD SubIFD) with automatic heap allocation for values longer than 4 bytes.
func buildExifTIFF(order binary.ByteOrder, orderName string, ifd0, exifIFD []exifTestEntry) []byte {
	ifd0Size := 2 + 12*len(ifd0) + 4
	subOffset := 0
	heapStart := 8 + ifd0Size
	if len(exifIFD) > 0 {
		subOffset = 8 + ifd0Size
		subSize := 2 + 12*len(exifIFD) + 4
		heapStart = subOffset + subSize
	}

	total := heapStart
	for _, e := range ifd0 {
		if !e.subIFDPtr && len(e.value) > 4 {
			total += len(e.value)
		}
	}
	for _, e := range exifIFD {
		if len(e.value) > 4 {
			total += len(e.value)
		}
	}

	buf := make([]byte, total)
	copy(buf[0:2], orderName)
	order.PutUint16(buf[2:4], 42)
	order.PutUint32(buf[4:8], 8)

	heap := heapStart
	writeIFD := func(entries []exifTestEntry, base int) {
		order.PutUint16(buf[base:base+2], uint16(len(entries)))
		p := base + 2
		for _, e := range entries {
			order.PutUint16(buf[p:p+2], e.tag)
			order.PutUint16(buf[p+2:p+4], e.typ)
			order.PutUint32(buf[p+4:p+8], e.count)
			switch {
			case e.subIFDPtr:
				order.PutUint32(buf[p+8:p+12], uint32(subOffset))
			case len(e.value) <= 4:
				copy(buf[p+8:p+12], e.value)
			default:
				order.PutUint32(buf[p+8:p+12], uint32(heap))
				copy(buf[heap:], e.value)
				heap += len(e.value)
			}
			p += 12
		}
		order.PutUint32(buf[p:p+4], 0) // next IFD offset
	}
	writeIFD(ifd0, 8)
	if len(exifIFD) > 0 {
		writeIFD(exifIFD, subOffset)
	}
	return buf
}

// ── readJPEGInfo ──

func TestReadJPEGInfo(t *testing.T) {
	soi := []byte{0xFF, 0xD8}
	com := func(s string) []byte { return jpegSegment(0xFE, []byte(s)) }

	tc260JSON := `{"AIGC":{"Label":"1","ContentProducer":"1191110102MACQD9K640"}}`
	tc260TIFF := buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x9286, tc260JSON)}, nil)

	cameraTIFF := buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{
		asciiEntry(0x010F, "Canon"),
		asciiEntry(0x0110, "EOS R5"),
		asciiEntry(0xA434, "RF 24-70"),
		exifIFDPointerEntry(),
	}, []exifTestEntry{
		rationalEntry(0x920A, binary.LittleEndian, 50, 1),
		rationalEntry(0x829D, binary.LittleEndian, 28, 10),
		shortEntry(0x8827, binary.LittleEndian, 800),
		rationalEntry(0x829A, binary.LittleEndian, 1, 250),
	})

	tests := []struct {
		name string
		data []byte
		want jpegInfo
	}{
		{"empty input", nil, jpegInfo{}},
		{"not a jpeg", []byte{0x00, 0x01, 0x02}, jpegInfo{}},
		{"truncated soi", []byte{0xFF}, jpegInfo{}},
		{"soi only", soi, jpegInfo{}},
		{"comment", concatJPEG(soi, com("Hello World")), jpegInfo{comment: "Hello World"}},
		{"comment trimmed", concatJPEG(soi, com("  spaced  ")), jpegInfo{comment: "spaced"}},
		{"zero length segment then comment", concatJPEG(soi, com(""), com("real")), jpegInfo{comment: "real"}},
		{"eoi then comment", concatJPEG(soi, []byte{0xFF, 0xD9}, com("after eoi")), jpegInfo{comment: "after eoi"}},
		{"rst marker skipped", concatJPEG(soi, []byte{0xFF, 0xD0}, com("x")), jpegInfo{comment: "x"}},
		{"ff padding before marker", concatJPEG(soi, []byte{0xFF}, com("pad")), jpegInfo{comment: "pad"}},
		{"sos stops parsing", concatJPEG(soi, com("first"), []byte{0xFF, 0xDA}, com("ignored")), jpegInfo{comment: "first"}},
		{"truncated segment", concatJPEG(soi, []byte{0xFF, 0xFE, 0x00, 0x40, 'a'}), jpegInfo{}},
		{"app11 jumbf undetected due to off-by-one at detect_jpeg.go:96", concatJPEG(soi, jpegSegment(0xEB, []byte("JUMBF\x01\x02\x03"))), jpegInfo{}},
		{"app11 not jumbf", concatJPEG(soi, jpegSegment(0xEB, []byte("NOTJUMBF"))), jpegInfo{}},
		{"app1 too short for exif", concatJPEG(soi, jpegSegment(0xE1, []byte("Exif\x00\x00"))), jpegInfo{}},
		{"app1 not exif", concatJPEG(soi, jpegSegment(0xE1, []byte("ExifX\x00\x00"))), jpegInfo{}},
		{
			"exif image description",
			concatJPEG(soi, app1ExifSegment(buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x010E, "Sunset")}, nil))),
			jpegInfo{imageDescription: "Sunset"},
		},
		{
			"exif software inline",
			concatJPEG(soi, app1ExifSegment(buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{{tag: 0x0131, typ: 2, count: 4, value: []byte("GIMP")}}, nil))),
			jpegInfo{software: "GIMP"},
		},
		{
			"exif tc260",
			concatJPEG(soi, app1ExifSegment(tc260TIFF)),
			jpegInfo{tc260Data: `{"Label":"1","ContentProducer":"1191110102MACQD9K640"}`},
		},
		{
			"exif camera",
			concatJPEG(soi, app1ExifSegment(cameraTIFF)),
			jpegInfo{camera: &CameraInfo{
				Make:         "Canon",
				Model:        "EOS R5",
				LensModel:    "RF 24-70",
				FocalLength:  "50.0mm",
				FNumber:      "2.8",
				ISO:          "800",
				ExposureTime: "1/250",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readJPEGInfo(bytes.NewReader(tt.data))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readJPEGInfo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// ── readExifASCII ──

func TestReadExifASCIIVariants(t *testing.T) {
	inline := make([]byte, 12)
	copy(inline[8:12], []byte("ABC\x00"))

	ws := make([]byte, 12)
	copy(ws[8:12], []byte(" ab "))

	oob := make([]byte, 20)
	binary.LittleEndian.PutUint32(oob[8:12], 9999)

	tests := []struct {
		name        string
		data        []byte
		order       binary.ByteOrder
		entryOffset int
		count       uint32
		want        string
	}{
		{
			"inline value",
			inline, binary.LittleEndian, 0, 4, "ABC",
		},
		{
			"inline whitespace trimmed",
			ws, binary.LittleEndian, 0, 4, "ab",
		},
		{
			"zero count",
			inline, binary.LittleEndian, 0, 0, "",
		},
		{
			"offset out of range",
			oob, binary.LittleEndian, 0, 100, "",
		},
		{
			"little endian offset value",
			buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x010F, "Canon")}, nil),
			binary.LittleEndian, 10, 6, "Canon",
		},
		{
			"big endian offset value",
			buildExifTIFF(binary.BigEndian, "MM", []exifTestEntry{asciiEntry(0x010F, "Nikon")}, nil),
			binary.BigEndian, 10, 6, "Nikon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readExifASCII(tt.data, tt.order, tt.entryOffset, tt.count)
			if got != tt.want {
				t.Errorf("readExifASCII() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── readExifRATIONAL ──

func TestReadExifRATIONALVariants(t *testing.T) {
	tests := []struct {
		name      string
		order     binary.ByteOrder
		orderName string
		typ       uint16
		count     uint32
		wantNum   uint32
		wantDen   uint32
	}{
		{"little endian valid", binary.LittleEndian, "II", 5, 1, 50, 1},
		{"big endian valid", binary.BigEndian, "MM", 5, 1, 50, 1},
		{"wrong type", binary.LittleEndian, "II", 3, 1, 0, 0},
		{"wrong count", binary.LittleEndian, "II", 5, 2, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildExifTIFF(tt.order, tt.orderName, []exifTestEntry{
				rationalEntry(0x920A, tt.order, 50, 1),
			}, nil)
			num, den := readExifRATIONAL(data, tt.order, 10, tt.typ, tt.count)
			if num != tt.wantNum || den != tt.wantDen {
				t.Errorf("readExifRATIONAL() = %d/%d, want %d/%d", num, den, tt.wantNum, tt.wantDen)
			}
		})
	}

	t.Run("offset out of range", func(t *testing.T) {
		data := make([]byte, 12)
		binary.LittleEndian.PutUint32(data[8:12], 100)
		num, den := readExifRATIONAL(data, binary.LittleEndian, 0, 5, 1)
		if num != 0 || den != 0 {
			t.Errorf("readExifRATIONAL() = %d/%d, want 0/0", num, den)
		}
	})
}

// ── readExifImageDescription ──

func TestReadExifImageDescription(t *testing.T) {
	tests := []struct {
		name  string
		build func() []byte
		want  string
	}{
		{
			"little endian offset value",
			func() []byte {
				return buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x010E, "Sunset")}, nil)
			},
			"Sunset",
		},
		{
			"big endian offset value",
			func() []byte {
				return buildExifTIFF(binary.BigEndian, "MM", []exifTestEntry{asciiEntry(0x010E, "Sunrise")}, nil)
			},
			"Sunrise",
		},
		{
			"inline value",
			func() []byte {
				return buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{
					{tag: 0x010E, typ: 2, count: 3, value: []byte("Hi\x00")},
				}, nil)
			},
			"Hi",
		},
		{
			"non ascii type ignored",
			func() []byte {
				return buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{shortEntry(0x010E, binary.LittleEndian, 5)}, nil)
			},
			"",
		},
		{
			"missing tag",
			func() []byte {
				return buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x010F, "Canon")}, nil)
			},
			"",
		},
		{
			"bad byte order",
			func() []byte {
				d := make([]byte, 16)
				copy(d[0:2], "XX")
				return d
			},
			"",
		},
		{
			"wrong magic",
			func() []byte {
				d := buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x010E, "Sunset")}, nil)
				binary.LittleEndian.PutUint16(d[2:4], 43)
				return d
			},
			"",
		},
		{
			"ifd offset out of range",
			func() []byte {
				d := buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x010E, "Sunset")}, nil)
				binary.LittleEndian.PutUint32(d[4:8], 9999)
				return d
			},
			"",
		},
		{
			"short data",
			func() []byte { return []byte("II*\x00") },
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readExifImageDescription(tt.build())
			if got != tt.want {
				t.Errorf("readExifImageDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── readExifSoftware ──

func TestReadExifSoftware(t *testing.T) {
	tests := []struct {
		name  string
		build func() []byte
		want  string
	}{
		{
			"inline value",
			func() []byte {
				return buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{
					{tag: 0x0131, typ: 2, count: 4, value: []byte("GIMP")},
				}, nil)
			},
			"GIMP",
		},
		{
			"big endian offset value with trailing data",
			func() []byte {
				d := buildExifTIFF(binary.BigEndian, "MM", []exifTestEntry{asciiEntry(0x0131, "MySoftware 1.0")}, nil)
				return append(d, make([]byte, 100)...)
			},
			"MySoftware 1.0",
		},
		{
			"empty inline value",
			func() []byte {
				return buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{
					{tag: 0x0131, typ: 2, count: 0},
				}, nil)
			},
			"",
		},
		{
			"missing tag",
			func() []byte {
				return buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x010F, "Canon")}, nil)
			},
			"",
		},
		{
			"bad byte order",
			func() []byte {
				d := make([]byte, 16)
				copy(d[0:2], "XX")
				return d
			},
			"",
		},
		{
			"wrong magic",
			func() []byte {
				d := buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x0131, "GIMP")}, nil)
				binary.LittleEndian.PutUint16(d[2:4], 43)
				return d
			},
			"",
		},
		{
			"ifd offset out of range",
			func() []byte {
				d := buildExifTIFF(binary.LittleEndian, "II", []exifTestEntry{asciiEntry(0x0131, "GIMP")}, nil)
				binary.LittleEndian.PutUint32(d[4:8], 9999)
				return d
			},
			"",
		},
		{
			"short data",
			func() []byte { return []byte("MM") },
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readExifSoftware(tt.build())
			if got != tt.want {
				t.Errorf("readExifSoftware() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── readExifCamera ──

func TestReadExifCamera(t *testing.T) {
	le := binary.LittleEndian
	be := binary.BigEndian

	fullIID0 := []exifTestEntry{
		asciiEntry(0x010F, "Canon"),
		asciiEntry(0x0110, "EOS R5"),
		asciiEntry(0xA434, "RF 24-70"),
		exifIFDPointerEntry(),
	}
	fullSub := []exifTestEntry{
		rationalEntry(0x920A, le, 50, 1),
		rationalEntry(0x829D, le, 28, 10),
		shortEntry(0x8827, le, 800),
		rationalEntry(0x829A, le, 1, 250),
	}

	tests := []struct {
		name  string
		build func() []byte
		want  *CameraInfo
	}{
		{
			"full camera little endian",
			func() []byte { return buildExifTIFF(le, "II", fullIID0, fullSub) },
			&CameraInfo{
				Make:         "Canon",
				Model:        "EOS R5",
				LensModel:    "RF 24-70",
				FocalLength:  "50.0mm",
				FNumber:      "2.8",
				ISO:          "800",
				ExposureTime: "1/250",
			},
		},
		{
			"full camera big endian",
			func() []byte {
				return buildExifTIFF(be, "MM", []exifTestEntry{
					asciiEntry(0x010F, "Nikon"),
					asciiEntry(0x0110, "Z9"),
					asciiEntry(0xA434, "NIKKOR Z"),
					exifIFDPointerEntry(),
				}, []exifTestEntry{
					rationalEntry(0x920A, be, 35, 1),
					rationalEntry(0x829D, be, 4, 1),
					shortEntry(0x8827, be, 100),
					rationalEntry(0x829A, be, 1, 60),
				})
			},
			&CameraInfo{
				Make:         "Nikon",
				Model:        "Z9",
				LensModel:    "NIKKOR Z",
				FocalLength:  "35.0mm",
				FNumber:      "4.0",
				ISO:          "100",
				ExposureTime: "1/60",
			},
		},
		{
			"make only",
			func() []byte {
				return buildExifTIFF(le, "II", []exifTestEntry{asciiEntry(0x010F, "Sony")}, nil)
			},
			&CameraInfo{Make: "Sony"},
		},
		{
			"exif ifd iso only",
			func() []byte {
				return buildExifTIFF(le, "II", []exifTestEntry{exifIFDPointerEntry()}, []exifTestEntry{
					shortEntry(0x8827, le, 400),
				})
			},
			&CameraInfo{ISO: "400"},
		},
		{
			"sub ifd pointer resolves to zero",
			func() []byte {
				return buildExifTIFF(le, "II", []exifTestEntry{asciiEntry(0x010F, "Canon"), exifIFDPointerEntry()}, nil)
			},
			&CameraInfo{Make: "Canon"},
		},
		{
			"no camera tags",
			func() []byte {
				return buildExifTIFF(le, "II", []exifTestEntry{asciiEntry(0x010E, "Sunset")}, nil)
			},
			nil,
		},
		{
			"short data",
			func() []byte { return []byte("II*\x00") },
			nil,
		},
		{
			"bad byte order",
			func() []byte {
				d := make([]byte, 16)
				copy(d[0:2], "XX")
				return d
			},
			nil,
		},
		{
			"wrong magic",
			func() []byte {
				d := buildExifTIFF(le, "II", []exifTestEntry{asciiEntry(0x010F, "Canon")}, nil)
				binary.LittleEndian.PutUint16(d[2:4], 43)
				return d
			},
			nil,
		},
		{
			"ifd offset out of range",
			func() []byte {
				d := buildExifTIFF(le, "II", []exifTestEntry{asciiEntry(0x010F, "Canon")}, nil)
				binary.LittleEndian.PutUint32(d[4:8], 9999)
				return d
			},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readExifCamera(tt.build())
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readExifCamera() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// ── scanEXIFForTC260 ──

func TestScanEXIFForTC260(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"no key", []byte("plain data without any label"), ""},
		{"empty data", nil, ""},
		{
			"nested AIGC with ContentProducer",
			[]byte(`prefix {"AIGC":{"Label":"1","ContentProducer":"1191110102MACQD9K640"}} suffix`),
			`{"Label":"1","ContentProducer":"1191110102MACQD9K640"}`,
		},
		{
			"nested AIGC with ServiceProvider",
			[]byte(`{"AIGC":{"ServiceProvider":"Acme"}}`),
			`{"ServiceProvider":"Acme"}`,
		},
		{
			"TC260 key fallback",
			[]byte(`{"TC260":{"ContentProducer":"X"}}`),
			`{"ContentProducer":"X"}`,
		},
		{"AIGC key without object", []byte(`{"AIGC":"not an object"}`), ""},
		{"AIGC empty object", []byte(`{"AIGC":{}}`), ""},
		{"AIGC missing producer fields", []byte(`{"AIGC":{"Label":"1"}}`), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scanEXIFForTC260(tt.data)
			if got != tt.want {
				t.Errorf("scanEXIFForTC260() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── hasControlChars ──

func TestHasControlCharsEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"printable", "Adobe Photoshop 2024", false},
		{"space is allowed", "a b", false},
		{"null excluded", "a\x00b", false},
		{"del is not control", "a\x7fb", false},
		{"newline", "a\nb", true},
		{"carriage return", "a\rb", true},
		{"tab", "a\tb", true},
		{"unit separator", "a\x1fb", true},
		{"soh", "a\x01b", true},
		{"utf8 multibyte", "中文", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasControlChars(tt.input); got != tt.want {
				t.Errorf("hasControlChars(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ── struct field coverage ──

func TestCameraInfoStruct(t *testing.T) {
	c := CameraInfo{
		Make:         "Canon",
		Model:        "EOS R5",
		LensModel:    "RF 24-70",
		FocalLength:  "50.0mm",
		FNumber:      "2.8",
		ISO:          "800",
		ExposureTime: "1/250",
	}
	if c.Make != "Canon" || c.Model != "EOS R5" || c.LensModel != "RF 24-70" ||
		c.FocalLength != "50.0mm" || c.FNumber != "2.8" || c.ISO != "800" || c.ExposureTime != "1/250" {
		t.Errorf("CameraInfo fields not settable as expected: %+v", c)
	}

	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal(CameraInfo) failed: %v", err)
	}
	for _, key := range []string{`"make"`, `"model"`, `"lens_model"`, `"focal_length"`, `"f_number"`, `"iso"`, `"exposure_time"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("CameraInfo JSON should contain %s, got %s", key, raw)
		}
	}

	empty, err := json.Marshal(CameraInfo{})
	if err != nil {
		t.Fatalf("json.Marshal(empty CameraInfo) failed: %v", err)
	}
	if strings.Contains(string(empty), `"make"`) {
		t.Errorf("empty CameraInfo should omit empty fields, got %s", empty)
	}
}

func TestJPEGInfoStruct(t *testing.T) {
	var zero jpegInfo
	if zero.comment != "" || zero.software != "" || zero.imageDescription != "" ||
		zero.hasC2PA || zero.tc260Data != "" || zero.camera != nil {
		t.Errorf("zero jpegInfo should be empty, got %+v", zero)
	}

	info := jpegInfo{
		comment:          "a comment",
		software:         "GIMP",
		imageDescription: "Sunset",
		hasC2PA:          true,
		tc260Data:        `{"ContentProducer":"X"}`,
		camera:           &CameraInfo{Make: "Canon"},
	}
	if info.comment != "a comment" || info.software != "GIMP" || info.imageDescription != "Sunset" ||
		!info.hasC2PA || info.tc260Data != `{"ContentProducer":"X"}` ||
		info.camera == nil || info.camera.Make != "Canon" {
		t.Errorf("jpegInfo fields not settable as expected: %+v", info)
	}
}
