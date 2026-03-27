package pebblestore

import (
	"encoding/binary"
	"sort"

	"github.com/cockroachdb/pebble"
)

// EvaluateAllCurrent scans all entity-current keys (0x03 prefix) and returns
// their IDs sorted in descending order.
func (s *PebbleStore) EvaluateAllCurrent(reader pebble.Reader) ([]uint64, error) {
	prefix := []byte{prefixEntityCurrent}
	iter, err := reader.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var ids []uint64
	for iter.First(); iter.Valid(); iter.Next() {
		val := iter.Value()
		if len(val) == 8 {
			ids = append(ids, binary.BigEndian.Uint64(val))
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	sort.Slice(ids, func(i, j int) bool {
		return ids[i] > ids[j]
	})

	return ids, nil
}

// GetNumberOfEntities counts the number of entity-current keys (0x03 prefix).
func (s *PebbleStore) GetNumberOfEntities(reader pebble.Reader) (uint64, error) {
	prefix := []byte{prefixEntityCurrent}
	iter, err := reader.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	var count uint64
	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}
	if err := iter.Error(); err != nil {
		return 0, err
	}

	return count, nil
}
