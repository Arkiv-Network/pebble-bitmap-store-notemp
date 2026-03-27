package store

import (
	"bytes"

	"github.com/RoaringBitmap/roaring/v2/roaring64"
)

type Bitmap struct {
	*roaring64.Bitmap
}

func NewBitmap() *Bitmap {
	return &Bitmap{Bitmap: roaring64.New()}
}

func (b *Bitmap) MarshalBinary() ([]byte, error) {
	if b.Bitmap == nil {
		return nil, nil
	}

	b.Bitmap.RunOptimize()

	buf := new(bytes.Buffer)
	_, err := b.Bitmap.WriteTo(buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
