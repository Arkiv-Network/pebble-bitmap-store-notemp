package pebblestore

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/cockroachdb/pebble/vfs"
)

// PebbleStore implements persistent storage backed by PebbleDB.
type PebbleStore struct {
	db        *pebble.DB
	log       *slog.Logger
	idMu      sync.Mutex
	nextIDVal uint64
}

// NewPebbleStore opens (or creates) a PebbleDB database at dbPath and returns
// a ready-to-use PebbleStore. The existing ID counter is loaded from the
// database so that new IDs continue from where the previous run left off.
func NewPebbleStore(log *slog.Logger, dbPath string) (*PebbleStore, error) {
	cache := pebble.NewCache(512 << 20)
	defer cache.Unref()

	levelOpts := func(compression pebble.Compression) pebble.LevelOptions {
		return pebble.LevelOptions{
			BlockSize:    32 << 10,
			Compression:  compression,
			FilterPolicy: bloom.FilterPolicy(10),
			FilterType:   pebble.TableFilter,
		}
	}

	opts := &pebble.Options{
		Cache:                    cache,
		MemTableSize:             64 << 20,
		MaxConcurrentCompactions: func() int { return 2 },
		BytesPerSync:             1 << 20,
		WALBytesPerSync:          1 << 20,
		Levels: []pebble.LevelOptions{
			levelOpts(pebble.SnappyCompression), // L0
			levelOpts(pebble.SnappyCompression), // L1
			levelOpts(pebble.SnappyCompression), // L2
			levelOpts(pebble.SnappyCompression), // L3
			levelOpts(pebble.SnappyCompression), // L4
			levelOpts(pebble.SnappyCompression), // L5
			levelOpts(pebble.ZstdCompression),   // L6
		},
	}

	if dbPath != "" {
		err := os.MkdirAll(dbPath, 0o755)
		if err != nil {
			return nil, fmt.Errorf("pebblestore: create directory: %w", err)
		}
	} else {
		opts.FS = vfs.NewMem()
	}

	db, err := pebble.Open(dbPath, opts)
	if err != nil {
		return nil, fmt.Errorf("pebblestore: open database: %w", err)
	}

	s := &PebbleStore{
		db:        db,
		log:       log,
		nextIDVal: 1,
	}

	// Load the persisted ID counter if it exists.
	val, closer, err := db.Get(idCounterKey())
	if err == nil {
		defer closer.Close()
		if len(val) == 8 {
			s.nextIDVal = binary.BigEndian.Uint64(val)
		}
	} else if err != pebble.ErrNotFound {
		_ = db.Close()
		return nil, fmt.Errorf("pebblestore: read id counter: %w", err)
	}

	// Migrate entity count if the key does not yet exist.
	_, closer, err = db.Get(entityCountKey())
	switch err {
	case nil:
		closer.Close()
	case pebble.ErrNotFound:
		count, countErr := s.countEntitiesByScan(db)
		if countErr != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pebblestore: migrate entity count: %w", countErr)
		}
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], count)
		if writeErr := db.Set(entityCountKey(), buf[:], pebble.Sync); writeErr != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pebblestore: persist migrated entity count: %w", writeErr)
		}
	default:
		_ = db.Close()
		return nil, fmt.Errorf("pebblestore: read entity count: %w", err)
	}

	log.Info("pebblestore opened", "path", dbPath, "nextID", s.nextIDVal)
	return s, nil
}

// DB returns the underlying PebbleDB instance for use as a pebble.Reader in
// tests and direct reads.
func (s *PebbleStore) DB() *pebble.DB {
	return s.db
}

// Close closes the underlying PebbleDB database.
func (s *PebbleStore) Close() error {
	return s.db.Close()
}

// GetLastBlock returns the last processed block number, or 0 if none has been
// recorded yet.
func (s *PebbleStore) GetLastBlock() (uint64, error) {
	val, closer, err := s.db.Get(lastBlockKey())
	if err == pebble.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("pebblestore: get last block: %w", err)
	}
	defer closer.Close()

	if len(val) != 8 {
		return 0, fmt.Errorf("pebblestore: last block value has unexpected length %d", len(val))
	}
	return binary.BigEndian.Uint64(val), nil
}

// UpsertLastBlock writes the block number to the last-block key in the given
// batch.
func (s *PebbleStore) UpsertLastBlock(batch *pebble.Batch, block uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], block)
	return batch.Set(lastBlockKey(), buf[:], pebble.Sync)
}

// GetNumberOfEntities reads the entity count from the db.
func (s *PebbleStore) GetNumberOfEntities() (uint64, error) {
	return s.getNumberOfEntities(s.db)
}

// GetNumberOfEntities reads the entity count from the db.
func (s *PebbleStore) getNumberOfEntities(r pebble.Reader) (uint64, error) {
	val, closer, err := r.Get(entityCountKey())
	if err == pebble.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("pebblestore: get entity count: %w", err)
	}
	defer closer.Close()
	if len(val) != 8 {
		return 0, fmt.Errorf("pebblestore: entity count value has unexpected length %d", len(val))
	}
	return binary.BigEndian.Uint64(val), nil
}

// countEntitiesByScan counts entity-current keys (0x03 prefix) by scanning.
// Used only during migration for databases created before entity count tracking.
func (s *PebbleStore) countEntitiesByScan(reader pebble.Reader) (uint64, error) {
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

// incrementEntityCount reads the current count from reader, increments it,
// and writes the new value to the batch.
func (s *PebbleStore) incrementEntityCount(batch *pebble.Batch, reader pebble.Reader) error {
	count, err := s.getNumberOfEntities(reader)
	if err != nil {
		return err
	}
	count++
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], count)
	return batch.Set(entityCountKey(), buf[:], pebble.Sync)
}

// decrementEntityCount reads the current count from reader, decrements it,
// and writes the new value to the batch.
func (s *PebbleStore) decrementEntityCount(batch *pebble.Batch, reader pebble.Reader) error {
	count, err := s.getNumberOfEntities(reader)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("pebblestore: entity count underflow")
	}
	count--
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], count)
	return batch.Set(entityCountKey(), buf[:], pebble.Sync)
}

// nextID atomically allocates a new unique ID and persists the updated counter
// to the provided batch. The caller must ensure the batch is eventually
// committed.
func (s *PebbleStore) nextID(batch *pebble.Batch) (uint64, error) {
	s.idMu.Lock()
	defer s.idMu.Unlock()

	id := s.nextIDVal
	s.nextIDVal++

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], s.nextIDVal)
	if err := batch.Set(idCounterKey(), buf[:], pebble.Sync); err != nil {
		// Roll back the in-memory counter on write failure.
		s.nextIDVal--
		return 0, fmt.Errorf("pebblestore: persist id counter: %w", err)
	}

	return id, nil
}
