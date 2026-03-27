package pebblestore

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/metrics"

	arkivevents "github.com/Arkiv-Network/arkiv-events"
	"github.com/Arkiv-Network/arkiv-events/events"
	"github.com/Arkiv-Network/sqlite-bitmap-store-notemp/store"
)

var (
	metricOperationStarted    = metrics.NewRegisteredCounter("arkiv_store/operations_started", nil)
	metricOperationSuccessful = metrics.NewRegisteredCounter("arkiv_store/operations_successful", nil)
	metricCreates             = metrics.NewRegisteredCounter("arkiv_store/creates", nil)
	metricCreatesBytes        = metrics.NewRegisteredCounter("arkiv_store/creates_bytes", nil)
	metricUpdates             = metrics.NewRegisteredCounter("arkiv_store/updates", nil)
	metricUpdatesBytes        = metrics.NewRegisteredCounter("arkiv_store/updates_bytes", nil)
	metricDeletes             = metrics.NewRegisteredCounter("arkiv_store/deletes", nil)
	metricDeletesBytes        = metrics.NewRegisteredCounter("arkiv_store/deletes_bytes", nil)
	metricExtends             = metrics.NewRegisteredCounter("arkiv_store/extends", nil)
	metricOwnerChanges        = metrics.NewRegisteredCounter("arkiv_store/owner_changes", nil)
	metricOperationTime       = metrics.NewRegisteredHistogram("arkiv_store/operation_time_ms", nil, metrics.NewExpDecaySample(100, 0.4))
)

type blockStats struct {
	creates      int64
	createsBytes int64
	updates      int64
	updatesBytes int64
	deletes      int64
	deletesBytes int64
	extends      int64
	ownerChanges int64
}

// FollowEvents consumes batches of block events from the iterator and applies
// them to the PebbleDB store. Each batch is processed within a single
// IndexedBatch (which supports read-your-own-writes) and committed atomically.
func (s *PebbleStore) FollowEvents(ctx context.Context, iterator arkivevents.BatchIterator) error {
	for batch := range iterator {
		if batch.Error != nil {
			return fmt.Errorf("failed to follow events: %w", batch.Error)
		}

		stats := make(map[uint64]*blockStats)

		err := func() error {
			pBatch := s.db.NewIndexedBatch()
			defer pBatch.Close()

			firstBlock := batch.Batch.Blocks[0].Number
			lastBlock := batch.Batch.Blocks[len(batch.Batch.Blocks)-1].Number
			s.log.Info("new batch", "firstBlock", firstBlock, "lastBlock", lastBlock)

			lastBlockFromDB, err := s.GetLastBlock()
			if err != nil {
				return fmt.Errorf("failed to get last block from database: %w", err)
			}

			cache := newBitmapCache(s, pBatch, pBatch)

			startTime := time.Now()
			metricOperationStarted.Inc(1)

		mainLoop:
			for _, block := range batch.Batch.Blocks {

				if block.Number <= lastBlockFromDB {
					s.log.Info("skipping block", "block", block.Number, "lastBlockFromDB", lastBlockFromDB)
					continue mainLoop
				}

				if _, ok := stats[block.Number]; !ok {
					stats[block.Number] = &blockStats{}
				}
				blockStat := stats[block.Number]

				updatesMap := map[common.Hash][]*events.OPUpdate{}
				for _, operation := range block.Operations {
					if operation.Update != nil {
						currentUpdates := updatesMap[operation.Update.Key]
						currentUpdates = append(currentUpdates, operation.Update)
						updatesMap[operation.Update.Key] = currentUpdates
					}
				}

			operationLoop:
				for _, operation := range block.Operations {

					switch {

					case operation.Create != nil:
						blockStat.creates++
						blockStat.createsBytes += int64(len(operation.Create.Content))

						stringAttributes := maps.Clone(operation.Create.StringAttributes)
						stringAttributes["$owner"] = strings.ToLower(operation.Create.Owner.Hex())
						stringAttributes["$creator"] = strings.ToLower(operation.Create.Owner.Hex())
						stringAttributes["$key"] = strings.ToLower(operation.Create.Key.Hex())

						untilBlock := block.Number + operation.Create.BTL
						numericAttributes := maps.Clone(operation.Create.NumericAttributes)
						numericAttributes["$expiration"] = uint64(untilBlock)
						numericAttributes["$createdAtBlock"] = uint64(block.Number)
						numericAttributes["$lastModifiedAtBlock"] = uint64(block.Number)

						sequence := block.Number<<32 | operation.TxIndex<<16 | operation.OpIndex
						numericAttributes["$sequence"] = sequence
						numericAttributes["$txIndex"] = uint64(operation.TxIndex)
						numericAttributes["$opIndex"] = uint64(operation.OpIndex)

						id, err := s.UpsertPayload(pBatch, pBatch, UpsertPayloadParams{
							EntityKey:         operation.Create.Key.Bytes(),
							Payload:           operation.Create.Content,
							ContentType:       operation.Create.ContentType,
							StringAttributes:  store.NewStringAttributes(stringAttributes),
							NumericAttributes: store.NewNumericAttributes(numericAttributes),
						})
						if err != nil {
							return fmt.Errorf("failed to insert payload %s at block %d txIndex %d opIndex %d: %w",
								operation.Create.Key.Hex(), block.Number, operation.TxIndex, operation.OpIndex, err)
						}

						for k, v := range stringAttributes {
							if err := cache.AddToStringBitmap(k, v, id); err != nil {
								return fmt.Errorf("failed to add string attribute value bitmap: %w", err)
							}
						}

						for k, v := range numericAttributes {
							switch k {
							case "$txIndex", "$opIndex":
								continue
							}
							if err := cache.AddToNumericBitmap(k, v, id); err != nil {
								return fmt.Errorf("failed to add numeric attribute value bitmap: %w", err)
							}
						}

					case operation.Update != nil:
						updates := updatesMap[operation.Update.Key]
						lastUpdate := updates[len(updates)-1]

						if operation.Update != lastUpdate {
							continue operationLoop
						}

						blockStat.updates++
						blockStat.updatesBytes += int64(len(operation.Update.Content))

						key := operation.Update.Key.Bytes()

						latestPayload, err := s.GetCurrentPayloadForEntityKey(pBatch, key)
						if err != nil {
							return fmt.Errorf("failed to get latest payload: %w", err)
						}

						oldStringAttributes := latestPayload.StringAttributes
						oldNumericAttributes := latestPayload.NumericAttributes

						stringAttributes := maps.Clone(operation.Update.StringAttributes)
						stringAttributes["$owner"] = strings.ToLower(operation.Update.Owner.Hex())
						stringAttributes["$creator"] = oldStringAttributes.Values["$creator"]
						stringAttributes["$key"] = strings.ToLower(operation.Update.Key.Hex())

						untilBlock := block.Number + operation.Update.BTL
						numericAttributes := maps.Clone(operation.Update.NumericAttributes)
						numericAttributes["$expiration"] = uint64(untilBlock)
						numericAttributes["$createdAtBlock"] = oldNumericAttributes.Values["$createdAtBlock"]
						numericAttributes["$sequence"] = oldNumericAttributes.Values["$sequence"]
						numericAttributes["$txIndex"] = oldNumericAttributes.Values["$txIndex"]
						numericAttributes["$opIndex"] = oldNumericAttributes.Values["$opIndex"]
						numericAttributes["$lastModifiedAtBlock"] = uint64(block.Number)

						id, err := s.UpsertPayload(pBatch, pBatch, UpsertPayloadParams{
							EntityKey:         key,
							Payload:           operation.Update.Content,
							ContentType:       operation.Update.ContentType,
							StringAttributes:  store.NewStringAttributes(stringAttributes),
							NumericAttributes: store.NewNumericAttributes(numericAttributes),
						})
						if err != nil {
							return fmt.Errorf("failed to insert payload 0x%x at block %d txIndex %d opIndex %d: %w",
								key, block.Number, operation.TxIndex, operation.OpIndex, err)
						}

						for k, v := range oldStringAttributes.Values {
							if err := cache.RemoveFromStringBitmap(k, v, id); err != nil {
								return fmt.Errorf("failed to remove string attribute value bitmap: %w", err)
							}
						}

						for k, v := range oldNumericAttributes.Values {
							switch k {
							case "$txIndex", "$opIndex":
								continue
							}
							if err := cache.RemoveFromNumericBitmap(k, v, id); err != nil {
								return fmt.Errorf("failed to remove numeric attribute value bitmap: %w", err)
							}
						}

						for k, v := range stringAttributes {
							if err := cache.AddToStringBitmap(k, v, id); err != nil {
								return fmt.Errorf("failed to add string attribute value bitmap: %w", err)
							}
						}

						for k, v := range numericAttributes {
							switch k {
							case "$txIndex", "$opIndex":
								continue
							}
							if err := cache.AddToNumericBitmap(k, v, id); err != nil {
								return fmt.Errorf("failed to add numeric attribute value bitmap: %w", err)
							}
						}

					case operation.Delete != nil || operation.Expire != nil:
						blockStat.deletes++

						var key []byte
						if operation.Delete != nil {
							key = common.Hash(*operation.Delete).Bytes()
						} else {
							key = common.Hash(*operation.Expire).Bytes()
						}

						latestPayload, err := s.GetCurrentPayloadForEntityKey(pBatch, key)
						if err != nil {
							return fmt.Errorf("failed to get latest payload: %w", err)
						}
						blockStat.deletesBytes += int64(len(latestPayload.Payload))

						for k, v := range latestPayload.StringAttributes.Values {
							if err := cache.RemoveFromStringBitmap(k, v, latestPayload.ID); err != nil {
								return fmt.Errorf("failed to remove string attribute value bitmap: %w", err)
							}
						}

						for k, v := range latestPayload.NumericAttributes.Values {
							switch k {
							case "$txIndex", "$opIndex":
								continue
							}
							if err := cache.RemoveFromNumericBitmap(k, v, latestPayload.ID); err != nil {
								return fmt.Errorf("failed to remove numeric attribute value bitmap: %w", err)
							}
						}

						if err := s.DeletePayloadForEntityKey(pBatch, pBatch, key); err != nil {
							return fmt.Errorf("failed to delete payload: %w", err)
						}

					case operation.ExtendBTL != nil:
						blockStat.extends++

						key := operation.ExtendBTL.Key.Bytes()

						latestPayload, err := s.GetCurrentPayloadForEntityKey(pBatch, key)
						if err != nil {
							return fmt.Errorf("failed to get latest payload: %w", err)
						}

						oldExpiration := latestPayload.NumericAttributes.Values["$expiration"]
						newToBlock := oldExpiration + operation.ExtendBTL.BTL

						numericAttributes := maps.Clone(latestPayload.NumericAttributes.Values)
						numericAttributes["$expiration"] = uint64(newToBlock)

						id, err := s.UpsertPayload(pBatch, pBatch, UpsertPayloadParams{
							EntityKey:         key,
							Payload:           latestPayload.Payload,
							ContentType:       latestPayload.ContentType,
							StringAttributes:  latestPayload.StringAttributes,
							NumericAttributes: store.NewNumericAttributes(numericAttributes),
						})
						if err != nil {
							return fmt.Errorf("failed to insert payload at block %d txIndex %d opIndex %d: %w",
								block.Number, operation.TxIndex, operation.OpIndex, err)
						}

						if err := cache.RemoveFromNumericBitmap("$expiration", oldExpiration, id); err != nil {
							return fmt.Errorf("failed to remove numeric attribute value bitmap: %w", err)
						}

						if err := cache.AddToNumericBitmap("$expiration", newToBlock, id); err != nil {
							return fmt.Errorf("failed to add numeric attribute value bitmap: %w", err)
						}

					case operation.ChangeOwner != nil:
						blockStat.ownerChanges++

						key := operation.ChangeOwner.Key.Bytes()

						latestPayload, err := s.GetCurrentPayloadForEntityKey(pBatch, key)
						if err != nil {
							return fmt.Errorf("failed to get latest payload: %w", err)
						}

						oldOwner := latestPayload.StringAttributes.Values["$owner"]
						newOwner := strings.ToLower(operation.ChangeOwner.Owner.Hex())

						latestPayload.StringAttributes.Values["$owner"] = newOwner

						id, err := s.UpsertPayload(pBatch, pBatch, UpsertPayloadParams{
							EntityKey:         key,
							Payload:           latestPayload.Payload,
							ContentType:       latestPayload.ContentType,
							StringAttributes:  latestPayload.StringAttributes,
							NumericAttributes: latestPayload.NumericAttributes,
						})
						if err != nil {
							return fmt.Errorf("failed to insert payload at block %d txIndex %d opIndex %d: %w",
								block.Number, operation.TxIndex, operation.OpIndex, err)
						}

						if err := cache.RemoveFromStringBitmap("$owner", oldOwner, id); err != nil {
							return fmt.Errorf("failed to remove string attribute value bitmap for owner: %w", err)
						}

						if err := cache.AddToStringBitmap("$owner", newOwner, id); err != nil {
							return fmt.Errorf("failed to add string attribute value bitmap for owner: %w", err)
						}

					default:
						return fmt.Errorf("unknown operation: %v", operation)
					}
				}

				s.log.Info("block updated",
					"block", block.Number,
					"creates", blockStat.creates,
					"updates", blockStat.updates,
					"deletes", blockStat.deletes,
					"extends", blockStat.extends,
					"ownerChanges", blockStat.ownerChanges)
			}

			err = s.UpsertLastBlock(pBatch, lastBlock)
			if err != nil {
				return fmt.Errorf("failed to upsert last block: %w", err)
			}

			err = cache.Flush()
			if err != nil {
				return fmt.Errorf("failed to flush bitmap cache: %w", err)
			}

			err = pBatch.Commit(pebble.Sync)
			if err != nil {
				return fmt.Errorf("failed to commit pebble batch: %w", err)
			}

			// Calculate batch totals for logging and update metrics per block.
			var (
				totalCreates      int64
				totalCreatesBytes int64
				totalUpdates      int64
				totalUpdatesBytes int64
				totalDeletes      int64
				totalDeletesBytes int64
				totalExtends      int64
				totalOwnerChanges int64
			)

			for _, block := range batch.Batch.Blocks {
				if stat, ok := stats[block.Number]; ok {
					totalCreates += stat.creates
					totalCreatesBytes += stat.createsBytes
					totalUpdates += stat.updates
					totalUpdatesBytes += stat.updatesBytes
					totalDeletes += stat.deletes
					totalDeletesBytes += stat.deletesBytes
					totalExtends += stat.extends
					totalOwnerChanges += stat.ownerChanges

					metricCreates.Inc(stat.creates)
					metricCreatesBytes.Inc(stat.createsBytes)
					metricUpdates.Inc(stat.updates)
					metricUpdatesBytes.Inc(stat.updatesBytes)
					metricDeletes.Inc(stat.deletes)
					metricDeletesBytes.Inc(stat.deletesBytes)
					metricExtends.Inc(stat.extends)
					metricOwnerChanges.Inc(stat.ownerChanges)
				}
			}

			metricOperationSuccessful.Inc(1)
			metricOperationTime.Update(time.Since(startTime).Milliseconds())

			s.log.Info("batch processed",
				"firstBlock", firstBlock,
				"lastBlock", lastBlock,
				"processingTime", time.Since(startTime).Milliseconds(),
				"creates", totalCreates,
				"createsBytes", totalCreatesBytes,
				"updates", totalUpdates,
				"updatesBytes", totalUpdatesBytes,
				"deletes", totalDeletes,
				"deletesBytes", totalDeletesBytes,
				"extends", totalExtends,
				"ownerChanges", totalOwnerChanges)

			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}
