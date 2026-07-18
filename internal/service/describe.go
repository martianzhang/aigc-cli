package service

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
	pngstructure "github.com/dsoprea/go-png-image-structure/v2"
)

// WriteDescription writes a caption/description into the image file metadata.
func WriteDescription(path, caption string) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return writeJpegDescription(path, caption)
	case ".png":
		return writePngDescription(path, caption)
	case ".webp":
		return fmt.Errorf("writing description to WebP is not yet supported")
	case ".heic", ".heif":
		return fmt.Errorf("writing description to HEIC is not yet supported")
	default:
		return fmt.Errorf("unsupported format: %s", ext)
	}
}

// ReadDescription reads the caption/description from image file metadata.
func ReadDescription(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return readJpegDescription(path)
	case ".png":
		return readPngDescription(path)
	default:
		return "", nil
	}
}

func readJpegDescription(path string) (string, error) {
	intfc, err := jpegstructure.NewJpegMediaParser().ParseFile(path)
	if err != nil {
		return "", fmt.Errorf("parse JPEG: %w", err)
	}
	sl := intfc.(*jpegstructure.SegmentList)
	ib, err := sl.ConstructExifBuilder()
	if err != nil {
		return "", nil // no EXIF, no description
	}
	if ib == nil {
		return "", nil
	}
	bt, err := ib.FindTagWithName("ImageDescription")
	if err != nil {
		return "", nil // tag not found
	}
	val := bt.Value()
	if val.IsBytes() {
		return string(val.Bytes()), nil
	}
	return val.String(), nil
}

func readPngDescription(path string) (string, error) {
	intfc, err := pngstructure.NewPngMediaParser().ParseFile(path)
	if err != nil {
		return "", fmt.Errorf("parse PNG: %w", err)
	}
	cs := intfc.(*pngstructure.ChunkSlice)
	ib, err := cs.ConstructExifBuilder()
	if err != nil {
		return "", nil
	}
	bt, err := ib.FindTagWithName("ImageDescription")
	if err != nil {
		return "", nil
	}
	val := bt.Value()
	if val.IsBytes() {
		return string(val.Bytes()), nil
	}
	return val.String(), nil
}

func setDesc(ib *exif.IfdBuilder, caption string) error {
	if caption != "" {
		return ib.SetStandardWithName("ImageDescription", []byte(caption))
	}
	return ib.SetStandardWithName("ImageDescription", []byte(""))
}

func writeJpegDescription(path, caption string) error {
	intfc, err := jpegstructure.NewJpegMediaParser().ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse JPEG: %w", err)
	}

	sl := intfc.(*jpegstructure.SegmentList)
	ib, err := sl.ConstructExifBuilder()
	if err != nil {
		return fmt.Errorf("construct EXIF: %w", err)
	}
	if ib == nil {
		return fmt.Errorf("no EXIF data in JPEG")
	}

	if err := setDesc(ib, caption); err != nil {
		return err
	}
	if err := sl.SetExif(ib); err != nil {
		return fmt.Errorf("set EXIF on JPEG: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	return sl.Write(f)
}

func writePngDescription(path, caption string) error {
	intfc, err := pngstructure.NewPngMediaParser().ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse PNG: %w", err)
	}

	cs := intfc.(*pngstructure.ChunkSlice)
	ib, err := cs.ConstructExifBuilder()
	if err != nil {
		ti := exif.NewTagIndex()
		ifdMapping := exifcommon.NewIfdMapping()
		if err := exifcommon.LoadStandardIfds(ifdMapping); err != nil {
			return fmt.Errorf("load IFD mapping: %w", err)
		}
		ib = exif.NewIfdBuilder(ifdMapping, ti, exifcommon.IfdStandardIfdIdentity, binary.BigEndian)
	}

	if err := setDesc(ib, caption); err != nil {
		return err
	}

	if err := cs.SetExif(ib); err != nil {
		return fmt.Errorf("set EXIF: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	return cs.WriteTo(f)
}
