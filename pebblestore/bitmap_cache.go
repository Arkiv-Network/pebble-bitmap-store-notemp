package pebblestore

import (
	"fmt"
	"runtime"

	"github.com/cockroachdb/pebble"
	"golang.org/x/sync/errgroup"

	"github.com/Arkiv-Network/sqlite-bitmap-store/store"
)

type nameValue[T any] struct {
	name  string
	value T
}

type bitmapCache struct {
	ps     *PebbleStore
	reader pebble.Reader
	batch  *pebble.Batch

	stringBitmaps  map[nameValue[string]]*store.Bitmap
	numericBitmaps map[nameValue[uint64]]*store.Bitmap
}

func newBitmapCache(ps *PebbleStore, reader pebble.Reader, batch *pebble.Batch) *bitmapCache {
	return &bitmapCache{
		ps:             ps,
		reader:         reader,
		batch:          batch,
		stringBitmaps:  make(map[nameValue[string]]*store.Bitmap),
		numericBitmaps: make(map[nameValue[uint64]]*store.Bitmap),
	}
}

func (c *bitmapCache) AddToStringBitmap(name string, value string, id uint64) error {
	k := nameValue[string]{name: name, value: value}
	bitmap, ok := c.stringBitmaps[k]
	if !ok {
		var err error
		bitmap, err = c.ps.GetStringBitmap(c.reader, name, value)
		if err != nil {
			return fmt.Errorf("bitmap cache: get string bitmap %q=%q: %w", name, value, err)
		}
		c.stringBitmaps[k] = bitmap
	}

	bitmap.Add(id)
	return nil
}

func (c *bitmapCache) RemoveFromStringBitmap(name string, value string, id uint64) error {
	k := nameValue[string]{name: name, value: value}
	bitmap, ok := c.stringBitmaps[k]
	if !ok {
		var err error
		bitmap, err = c.ps.GetStringBitmap(c.reader, name, value)
		if err != nil {
			return fmt.Errorf("bitmap cache: get string bitmap %q=%q: %w", name, value, err)
		}
		c.stringBitmaps[k] = bitmap
	}

	bitmap.Remove(id)
	return nil
}

func (c *bitmapCache) AddToNumericBitmap(name string, value uint64, id uint64) error {
	k := nameValue[uint64]{name: name, value: value}
	bitmap, ok := c.numericBitmaps[k]
	if !ok {
		var err error
		bitmap, err = c.ps.GetNumericBitmap(c.reader, name, value)
		if err != nil {
			return fmt.Errorf("bitmap cache: get numeric bitmap %q=%d: %w", name, value, err)
		}
		c.numericBitmaps[k] = bitmap
	}

	bitmap.Add(id)
	return nil
}

func (c *bitmapCache) RemoveFromNumericBitmap(name string, value uint64, id uint64) error {
	k := nameValue[uint64]{name: name, value: value}
	bitmap, ok := c.numericBitmaps[k]
	if !ok {
		var err error
		bitmap, err = c.ps.GetNumericBitmap(c.reader, name, value)
		if err != nil {
			return fmt.Errorf("bitmap cache: get numeric bitmap %q=%d: %w", name, value, err)
		}
		c.numericBitmaps[k] = bitmap
	}

	bitmap.Remove(id)
	return nil
}

func (c *bitmapCache) Flush() error {
	eg := &errgroup.Group{}
	eg.SetLimit(runtime.NumCPU())

	for _, bitmap := range c.stringBitmaps {
		if bitmap.IsEmpty() {
			continue
		}
		eg.Go(func() error {
			bitmap.RunOptimize()
			return nil
		})
	}

	for _, bitmap := range c.numericBitmaps {
		if bitmap.IsEmpty() {
			continue
		}
		eg.Go(func() error {
			bitmap.RunOptimize()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("bitmap cache: run optimize: %w", err)
	}

	for k, bitmap := range c.stringBitmaps {
		if bitmap.IsEmpty() {
			if err := c.ps.DeleteStringBitmap(c.batch, k.name, k.value); err != nil {
				return fmt.Errorf("bitmap cache: delete string bitmap %q=%q: %w", k.name, k.value, err)
			}
			continue
		}
		if err := c.ps.SetStringBitmap(c.batch, k.name, k.value, bitmap); err != nil {
			return fmt.Errorf("bitmap cache: set string bitmap %q=%q: %w", k.name, k.value, err)
		}
	}

	for k, bitmap := range c.numericBitmaps {
		if bitmap.IsEmpty() {
			if err := c.ps.DeleteNumericBitmap(c.batch, k.name, k.value); err != nil {
				return fmt.Errorf("bitmap cache: delete numeric bitmap %q=%d: %w", k.name, k.value, err)
			}
			continue
		}
		if err := c.ps.SetNumericBitmap(c.batch, k.name, k.value, bitmap); err != nil {
			return fmt.Errorf("bitmap cache: set numeric bitmap %q=%d: %w", k.name, k.value, err)
		}
	}

	return nil
}
