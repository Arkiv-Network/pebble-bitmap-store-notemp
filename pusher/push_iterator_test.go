package pusher_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	arkivevents "github.com/Arkiv-Network/arkiv-events"
	"github.com/Arkiv-Network/arkiv-events/events"
	"github.com/Arkiv-Network/sqlite-bitmap-store-notemp/pebblestore"
	"github.com/Arkiv-Network/sqlite-bitmap-store-notemp/pusher"
)

var _ = Describe("PushIterator", func() {
	var (
		store  *pebblestore.PebbleStore
		tmpDir string
		ctx    context.Context
		cancel context.CancelFunc
		logger *slog.Logger
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "push_iterator_test")
		Expect(err).NotTo(HaveOccurred())

		logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err = pebblestore.NewPebbleStore(logger, dbPath)
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if store != nil {
			store.Close()
		}
		os.RemoveAll(tmpDir)
	})

	Describe("Push and FollowEvents integration", func() {
		It("should store a single create operation", func() {
			iterator := pusher.NewPushIterator()

			key := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			batch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:         key,
									ContentType: "application/json",
									BTL:         1000,
									Owner:       owner,
									Content:     []byte(`{"name": "test"}`),
									StringAttributes: map[string]string{
										"type": "document",
									},
									NumericAttributes: map[string]uint64{
										"version": 1,
									},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, batch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			lastBlock, err := store.GetLastBlock()
			Expect(err).NotTo(HaveOccurred())
			Expect(lastBlock).To(Equal(uint64(100)))

			row, err := store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row.Payload).To(Equal([]byte(`{"name": "test"}`)))
			Expect(row.ContentType).To(Equal("application/json"))
		})

		It("should store multiple blocks in a single batch", func() {
			iterator := pusher.NewPushIterator()

			key1 := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
			key2 := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			batch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key1,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("first entity"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
					{
						Number: 101,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key2,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("second entity"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, batch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			lastBlock, err := store.GetLastBlock()
			Expect(err).NotTo(HaveOccurred())
			Expect(lastBlock).To(Equal(uint64(101)))

			row1, err := store.GetCurrentPayloadForEntityKey(store.DB(), key1.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row1.Payload).To(Equal([]byte("first entity")))

			row2, err := store.GetCurrentPayloadForEntityKey(store.DB(), key2.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row2.Payload).To(Equal([]byte("second entity")))
		})

		It("should handle update operations", func() {
			iterator := pusher.NewPushIterator()

			key := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			createBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("original content"),
									StringAttributes:  map[string]string{"status": "draft"},
									NumericAttributes: map[string]uint64{"version": 1},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, createBatch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			updateIterator := pusher.NewPushIterator()

			updateBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 101,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Update: &events.OPUpdate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("updated content"),
									StringAttributes:  map[string]string{"status": "published"},
									NumericAttributes: map[string]uint64{"version": 2},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				updateIterator.Push(ctx, updateBatch)
				updateIterator.Close()
			}()

			err = store.FollowEvents(ctx, arkivevents.BatchIterator(updateIterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			row, err := store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row.Payload).To(Equal([]byte("updated content")))
			Expect(row.StringAttributes.Values["status"]).To(Equal("published"))
			Expect(row.NumericAttributes.Values["version"]).To(Equal(uint64(2)))
		})

		It("should handle delete operations", func() {
			iterator := pusher.NewPushIterator()

			key := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			createBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("to be deleted"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, createBatch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			deleteIterator := pusher.NewPushIterator()
			deleteKey := events.OPDelete(key)

			deleteBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 101,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Delete:  &deleteKey,
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				deleteIterator.Push(ctx, deleteBatch)
				deleteIterator.Close()
			}()

			err = store.FollowEvents(ctx, arkivevents.BatchIterator(deleteIterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			_, err = store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).To(HaveOccurred())
		})

		It("should handle extend BTL operations", func() {
			iterator := pusher.NewPushIterator()

			key := common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			createBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("content"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, createBatch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			row, err := store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row.NumericAttributes.Values["$expiration"]).To(Equal(uint64(600)))

			extendIterator := pusher.NewPushIterator()

			extendBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 200,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								ExtendBTL: &events.OPExtendBTL{
									Key: key,
									BTL: 1000,
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				extendIterator.Push(ctx, extendBatch)
				extendIterator.Close()
			}()

			err = store.FollowEvents(ctx, arkivevents.BatchIterator(extendIterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			row, err = store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row.NumericAttributes.Values["$expiration"]).To(Equal(uint64(1600)))
		})

		It("should handle change owner operations", func() {
			iterator := pusher.NewPushIterator()

			key := common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
			originalOwner := common.HexToAddress("0x1111111111111111111111111111111111111111")
			newOwner := common.HexToAddress("0x2222222222222222222222222222222222222222")

			createBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             originalOwner,
									Content:           []byte("content"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, createBatch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			row, err := store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row.StringAttributes.Values["$owner"]).To(Equal(strings.ToLower(originalOwner.Hex())))

			changeOwnerIterator := pusher.NewPushIterator()

			changeOwnerBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 101,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								ChangeOwner: &events.OPChangeOwner{
									Key:   key,
									Owner: newOwner,
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				changeOwnerIterator.Push(ctx, changeOwnerBatch)
				changeOwnerIterator.Close()
			}()

			err = store.FollowEvents(ctx, arkivevents.BatchIterator(changeOwnerIterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			row, err = store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row.StringAttributes.Values["$owner"]).To(Equal(strings.ToLower(newOwner.Hex())))
			Expect(row.StringAttributes.Values["$creator"]).To(Equal(strings.ToLower(originalOwner.Hex())))
		})

		It("should handle multiple batches pushed sequentially", func() {
			iterator := pusher.NewPushIterator()

			key1 := common.HexToHash("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
			key2 := common.HexToHash("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			batch1 := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key1,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("batch 1"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			batch2 := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 101,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key2,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("batch 2"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, batch1)
				iterator.Push(ctx, batch2)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			lastBlock, err := store.GetLastBlock()
			Expect(err).NotTo(HaveOccurred())
			Expect(lastBlock).To(Equal(uint64(101)))

			row1, err := store.GetCurrentPayloadForEntityKey(store.DB(), key1.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row1.Payload).To(Equal([]byte("batch 1")))

			row2, err := store.GetCurrentPayloadForEntityKey(store.DB(), key2.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row2.Payload).To(Equal([]byte("batch 2")))
		})

		It("should skip already processed blocks", func() {
			iterator := pusher.NewPushIterator()

			key := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			batch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("first"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, batch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			replayIterator := pusher.NewPushIterator()

			replayBatch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 0,
								OpIndex: 0,
								Create: &events.OPCreate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("should be ignored"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				replayIterator.Push(ctx, replayBatch)
				replayIterator.Close()
			}()

			err = store.FollowEvents(ctx, arkivevents.BatchIterator(replayIterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			row, err := store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(row.Payload).To(Equal([]byte("first")))
		})

		It("should set system attributes correctly on create", func() {
			iterator := pusher.NewPushIterator()

			key := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002")
			owner := common.HexToAddress("0x1234567890123456789012345678901234567890")

			batch := events.BlockBatch{
				Blocks: []events.Block{
					{
						Number: 100,
						Operations: []events.Operation{
							{
								TxIndex: 5,
								OpIndex: 3,
								Create: &events.OPCreate{
									Key:               key,
									ContentType:       "text/plain",
									BTL:               500,
									Owner:             owner,
									Content:           []byte("content"),
									StringAttributes:  map[string]string{},
									NumericAttributes: map[string]uint64{},
								},
							},
						},
					},
				},
			}

			go func() {
				defer GinkgoRecover()
				iterator.Push(ctx, batch)
				iterator.Close()
			}()

			err := store.FollowEvents(ctx, arkivevents.BatchIterator(iterator.Iterator()))
			Expect(err).NotTo(HaveOccurred())

			row, err := store.GetCurrentPayloadForEntityKey(store.DB(), key.Bytes())
			Expect(err).NotTo(HaveOccurred())

			Expect(row.StringAttributes.Values["$owner"]).To(Equal(strings.ToLower(owner.Hex())))
			Expect(row.StringAttributes.Values["$creator"]).To(Equal(strings.ToLower(owner.Hex())))
			Expect(row.StringAttributes.Values["$key"]).To(Equal(strings.ToLower(key.Hex())))

			Expect(row.NumericAttributes.Values["$expiration"]).To(Equal(uint64(600)))
			Expect(row.NumericAttributes.Values["$createdAtBlock"]).To(Equal(uint64(100)))
			Expect(row.NumericAttributes.Values["$lastModifiedAtBlock"]).To(Equal(uint64(100)))
			Expect(row.NumericAttributes.Values["$txIndex"]).To(Equal(uint64(5)))
			Expect(row.NumericAttributes.Values["$opIndex"]).To(Equal(uint64(3)))

			expectedSequence := uint64(100)<<32 | uint64(5)<<16 | uint64(3)
			Expect(row.NumericAttributes.Values["$sequence"]).To(Equal(expectedSequence))
		})
	})
})
