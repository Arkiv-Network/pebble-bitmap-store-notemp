package pebblestore

import (
	"fmt"

	"github.com/cockroachdb/pebble"

	"github.com/Arkiv-Network/pebble-bitmap-store-notemp/store"
)

// GetStringBitmap returns the bitmap for a (name, value) pair, or an empty
// bitmap if the key does not exist.
func (s *PebbleStore) GetStringBitmap(reader pebble.Reader, name, value string) (*store.Bitmap, error) {
	key := stringBitmapKey(name, value)

	val, closer, err := reader.Get(key)
	if err == pebble.ErrNotFound {
		return store.NewBitmap(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("pebblestore: get string bitmap %q=%q: %w", name, value, err)
	}
	data := make([]byte, len(val))
	copy(data, val)
	closer.Close()

	bm := store.NewBitmap()
	if err := bm.Bitmap.UnmarshalBinary(data); err != nil {
		return nil, fmt.Errorf("pebblestore: unmarshal string bitmap %q=%q: %w", name, value, err)
	}
	return bm, nil
}

// SetStringBitmap writes the bitmap for a (name, value) pair.
func (s *PebbleStore) SetStringBitmap(batch *pebble.Batch, name, value string, bm *store.Bitmap) error {
	key := stringBitmapKey(name, value)

	data, err := bm.MarshalBinary()
	if err != nil {
		return fmt.Errorf("pebblestore: marshal string bitmap %q=%q: %w", name, value, err)
	}
	if err := batch.Set(key, data, pebble.Sync); err != nil {
		return fmt.Errorf("pebblestore: set string bitmap %q=%q: %w", name, value, err)
	}
	return nil
}

// DeleteStringBitmap removes the bitmap for a (name, value) pair.
func (s *PebbleStore) DeleteStringBitmap(batch *pebble.Batch, name, value string) error {
	key := stringBitmapKey(name, value)
	if err := batch.Delete(key, pebble.Sync); err != nil {
		return fmt.Errorf("pebblestore: delete string bitmap %q=%q: %w", name, value, err)
	}
	return nil
}

// GetNumericBitmap returns the bitmap for a (name, value) pair, or an empty
// bitmap if the key does not exist.
func (s *PebbleStore) GetNumericBitmap(reader pebble.Reader, name string, value uint64) (*store.Bitmap, error) {
	key := numericBitmapKey(name, value)

	val, closer, err := reader.Get(key)
	if err == pebble.ErrNotFound {
		return store.NewBitmap(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("pebblestore: get numeric bitmap %q=%d: %w", name, value, err)
	}
	data := make([]byte, len(val))
	copy(data, val)
	closer.Close()

	bm := store.NewBitmap()
	if err := bm.Bitmap.UnmarshalBinary(data); err != nil {
		return nil, fmt.Errorf("pebblestore: unmarshal numeric bitmap %q=%d: %w", name, value, err)
	}
	return bm, nil
}

// SetNumericBitmap writes the bitmap for a (name, value) pair.
func (s *PebbleStore) SetNumericBitmap(batch *pebble.Batch, name string, value uint64, bm *store.Bitmap) error {
	key := numericBitmapKey(name, value)

	data, err := bm.MarshalBinary()
	if err != nil {
		return fmt.Errorf("pebblestore: marshal numeric bitmap %q=%d: %w", name, value, err)
	}
	if err := batch.Set(key, data, pebble.Sync); err != nil {
		return fmt.Errorf("pebblestore: set numeric bitmap %q=%d: %w", name, value, err)
	}
	return nil
}

// DeleteNumericBitmap removes the bitmap for a (name, value) pair.
func (s *PebbleStore) DeleteNumericBitmap(batch *pebble.Batch, name string, value uint64) error {
	key := numericBitmapKey(name, value)
	if err := batch.Delete(key, pebble.Sync); err != nil {
		return fmt.Errorf("pebblestore: delete numeric bitmap %q=%d: %w", name, value, err)
	}
	return nil
}
