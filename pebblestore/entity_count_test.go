package pebblestore_test

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/cockroachdb/pebble"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Arkiv-Network/pebble-bitmap-store-notemp/pebblestore"
	"github.com/Arkiv-Network/pebble-bitmap-store-notemp/store"
)

// entityKey returns a 32-byte key with the given byte repeated.
func entityKey(b byte) []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = b
	}
	return key
}

func makeParams(key []byte) pebblestore.UpsertPayloadParams {
	return pebblestore.UpsertPayloadParams{
		EntityKey:         key,
		Payload:           []byte("payload"),
		ContentType:       "text/plain",
		StringAttributes:  store.NewStringAttributes(map[string]string{}),
		NumericAttributes: store.NewNumericAttributes(map[string]uint64{}),
	}
}

var _ = Describe("Entity Count", func() {
	var (
		s      *pebblestore.PebbleStore
		logger *slog.Logger
	)

	logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))

	Describe("in-memory store", func() {
		BeforeEach(func() {
			var err error
			s, err = pebblestore.NewPebbleStore(logger, "")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if s != nil {
				s.Close()
			}
		})

		It("should start at zero for a fresh store", func() {
			Expect(s.GetNumberOfEntities()).To(Equal(uint64(0)))
		})

		It("should increment on new entity creation", func() {
			batch := s.DB().NewIndexedBatch()
			defer batch.Close()

			_, err := s.UpsertPayload(batch, batch, makeParams(entityKey(0x01)))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(1)))

			batch2 := s.DB().NewIndexedBatch()
			defer batch2.Close()

			_, err = s.UpsertPayload(batch2, batch2, makeParams(entityKey(0x02)))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch2.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(2)))
		})

		It("should not increment on entity update (same key)", func() {
			batch := s.DB().NewIndexedBatch()
			defer batch.Close()

			_, err := s.UpsertPayload(batch, batch, makeParams(entityKey(0x01)))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(1)))

			// Update same entity with different payload.
			batch2 := s.DB().NewIndexedBatch()
			defer batch2.Close()

			params := makeParams(entityKey(0x01))
			params.Payload = []byte("updated payload")
			_, err = s.UpsertPayload(batch2, batch2, params)
			Expect(err).NotTo(HaveOccurred())
			Expect(batch2.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(1)))
		})

		It("should decrement on delete", func() {
			batch := s.DB().NewIndexedBatch()
			defer batch.Close()

			_, err := s.UpsertPayload(batch, batch, makeParams(entityKey(0x01)))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(1)))

			batch2 := s.DB().NewIndexedBatch()
			defer batch2.Close()

			err = s.DeletePayloadForEntityKey(batch2, batch2, entityKey(0x01))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch2.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(0)))
		})

		It("should handle mixed create/update/delete operations", func() {
			// Create 3 entities.
			for _, b := range []byte{0x01, 0x02, 0x03} {
				batch := s.DB().NewIndexedBatch()
				_, err := s.UpsertPayload(batch, batch, makeParams(entityKey(b)))
				Expect(err).NotTo(HaveOccurred())
				Expect(batch.Commit(pebble.Sync)).To(Succeed())
			}
			Expect(s.GetNumberOfEntities()).To(Equal(uint64(3)))

			// Update entity 0x01.
			batch := s.DB().NewIndexedBatch()
			params := makeParams(entityKey(0x01))
			params.Payload = []byte("updated")
			_, err := s.UpsertPayload(batch, batch, params)
			Expect(err).NotTo(HaveOccurred())
			Expect(batch.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(3)))

			// Delete entity 0x02.
			batch2 := s.DB().NewIndexedBatch()
			err = s.DeletePayloadForEntityKey(batch2, batch2, entityKey(0x02))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch2.Commit(pebble.Sync)).To(Succeed())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(2)))
		})

		It("should handle multiple creates and deletes back to zero", func() {
			keys := []byte{0x01, 0x02, 0x03, 0x04, 0x05}

			for _, b := range keys {
				batch := s.DB().NewIndexedBatch()
				_, err := s.UpsertPayload(batch, batch, makeParams(entityKey(b)))
				Expect(err).NotTo(HaveOccurred())
				Expect(batch.Commit(pebble.Sync)).To(Succeed())
			}
			Expect(s.GetNumberOfEntities()).To(Equal(uint64(5)))

			for _, b := range keys {
				batch := s.DB().NewIndexedBatch()
				err := s.DeletePayloadForEntityKey(batch, batch, entityKey(b))
				Expect(err).NotTo(HaveOccurred())
				Expect(batch.Commit(pebble.Sync)).To(Succeed())
			}
			Expect(s.GetNumberOfEntities()).To(Equal(uint64(0)))
		})
	})

	Describe("persistence across restarts", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "entity_count_test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if s != nil {
				s.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("should persist count across store reopen", func() {
			dbPath := filepath.Join(tmpDir, "test.db")

			var err error
			s, err = pebblestore.NewPebbleStore(logger, dbPath)
			Expect(err).NotTo(HaveOccurred())

			for _, b := range []byte{0x01, 0x02, 0x03} {
				batch := s.DB().NewIndexedBatch()
				_, err := s.UpsertPayload(batch, batch, makeParams(entityKey(b)))
				Expect(err).NotTo(HaveOccurred())
				Expect(batch.Commit(pebble.Sync)).To(Succeed())
			}
			Expect(s.GetNumberOfEntities()).To(Equal(uint64(3)))

			s.Close()

			s, err = pebblestore.NewPebbleStore(logger, dbPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(3)))
		})

		It("should migrate count from existing database without count key", func() {
			dbPath := filepath.Join(tmpDir, "migrate.db")

			var err error
			s, err = pebblestore.NewPebbleStore(logger, dbPath)
			Expect(err).NotTo(HaveOccurred())

			for _, b := range []byte{0x01, 0x02} {
				batch := s.DB().NewIndexedBatch()
				_, err := s.UpsertPayload(batch, batch, makeParams(entityKey(b)))
				Expect(err).NotTo(HaveOccurred())
				Expect(batch.Commit(pebble.Sync)).To(Succeed())
			}
			Expect(s.GetNumberOfEntities()).To(Equal(uint64(2)))

			// Delete the entity count key to simulate a pre-migration database.
			err = s.DB().Delete([]byte{0x07}, pebble.Sync)
			Expect(err).NotTo(HaveOccurred())

			s.Close()

			// Reopen — should migrate by scanning 0x03 prefix.
			s, err = pebblestore.NewPebbleStore(logger, dbPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.GetNumberOfEntities()).To(Equal(uint64(2)))
		})
	})
})
