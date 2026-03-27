package pebblestore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Arkiv-Network/sqlite-bitmap-store/query"
	"github.com/RoaringBitmap/roaring/v2/roaring64"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ---------------------------------------------------------------------------
// Exported types
// ---------------------------------------------------------------------------

// QueryResultCountLimit is the maximum number of results returned per page.
const QueryResultCountLimit uint64 = 200

// IncludeData controls which entity fields are included in the query response.
type IncludeData struct {
	Key                         bool `json:"key"`
	Attributes                  bool `json:"attributes"`
	SyntheticAttributes         bool `json:"syntheticAttributes"`
	Payload                     bool `json:"payload"`
	ContentType                 bool `json:"contentType"`
	Expiration                  bool `json:"expiration"`
	Creator                     bool `json:"creator"`
	Owner                       bool `json:"owner"`
	CreatedAtBlock              bool `json:"createdAtBlock"`
	LastModifiedAtBlock         bool `json:"lastModifiedAtBlock"`
	TransactionIndexInBlock     bool `json:"transactionIndexInBlock"`
	OperationIndexInTransaction bool `json:"operationIndexInTransaction"`
}

// Options controls query behaviour such as pagination and field selection.
type Options struct {
	AtBlock        *hexutil.Uint64 `json:"atBlock,omitempty"`
	IncludeData    *IncludeData    `json:"includeData,omitempty"`
	ResultsPerPage *hexutil.Uint64 `json:"resultsPerPage,omitempty"`
	Cursor         string          `json:"cursor,omitempty"`
}

// GetAtBlock returns the requested block number, or 0 if unset.
func (o *Options) GetAtBlock() uint64 {
	if o == nil || o.AtBlock == nil {
		return 0
	}
	return uint64(*o.AtBlock)
}

// GetResultsPerPage returns the requested page size, clamped to
// QueryResultCountLimit.
func (o *Options) GetResultsPerPage() uint64 {
	if o == nil || o.ResultsPerPage == nil || uint64(*o.ResultsPerPage) > QueryResultCountLimit {
		return QueryResultCountLimit
	}
	return uint64(*o.ResultsPerPage)
}

// GetIncludeData returns the field-inclusion settings, using sensible defaults
// when the caller did not supply any.
func (o *Options) GetIncludeData() IncludeData {
	if o == nil || o.IncludeData == nil {
		return IncludeData{
			Key:         true,
			ContentType: true,
			Payload:     true,
			Creator:     true,
			Owner:       true,
			Attributes:  true,
			Expiration:  true,
		}
	}
	return *o.IncludeData
}

// GetCursor decodes the hex-encoded cursor string, returning nil when no
// cursor is present.
func (o *Options) GetCursor() (*uint64, error) {
	if o == nil || o.Cursor == "" {
		return nil, nil
	}

	cursor, err := hexutil.DecodeUint64(o.Cursor)
	if err != nil {
		return nil, fmt.Errorf("error decoding cursor: %w", err)
	}

	return &cursor, nil
}

// QueryResponse is returned by QueryEntities.
type QueryResponse struct {
	Data        []json.RawMessage `json:"data"`
	BlockNumber hexutil.Uint64    `json:"blockNumber"`
	Cursor      *string           `json:"cursor,omitempty"`
}

// EntityData represents a single entity in the query response.
type EntityData struct {
	Key                         *common.Hash    `json:"key,omitempty"`
	Value                       hexutil.Bytes   `json:"value,omitempty"`
	ContentType                 *string         `json:"contentType,omitempty"`
	ExpiresAt                   *uint64         `json:"expiresAt,omitempty"`
	Creator                     *common.Address `json:"creator,omitempty"`
	Owner                       *common.Address `json:"owner,omitempty"`
	CreatedAtBlock              *uint64         `json:"createdAtBlock,omitempty"`
	LastModifiedAtBlock         *uint64         `json:"lastModifiedAtBlock,omitempty"`
	TransactionIndexInBlock     *uint64         `json:"transactionIndexInBlock,omitempty"`
	OperationIndexInTransaction *uint64         `json:"operationIndexInTransaction,omitempty"`

	StringAttributes  []Attribute[string] `json:"stringAttributes,omitempty"`
	NumericAttributes []Attribute[uint64] `json:"numericAttributes,omitempty"`
}

// Attribute is a generic key/value pair used for both string and numeric
// attributes.
type Attribute[T any] struct {
	Key   string `json:"key"`
	Value T      `json:"value"`
}

// ---------------------------------------------------------------------------
// QueryEntities
// ---------------------------------------------------------------------------

const maxResultBytes = 512 * 1024 * 1024

// QueryEntities evaluates a query string against the store and returns
// matching entities with pagination support.
func (s *PebbleStore) QueryEntities(
	ctx context.Context,
	queryStr string,
	options *Options,
) (*QueryResponse, error) {

	res := &QueryResponse{
		Data:        []json.RawMessage{},
		BlockNumber: 0,
		Cursor:      nil,
	}

	// 1. Wait until the store has processed at least the requested block.
	{
		timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		for {
			lastBlock, err := s.GetLastBlock()
			if err != nil {
				return nil, fmt.Errorf("error getting last block: %w", err)
			}
			if lastBlock >= options.GetAtBlock() {
				break
			}
			select {
			case <-timeoutCtx.Done():
				return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		cancel()
	}

	// 2. Parse the query.
	q, err := query.Parse(queryStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing query: %w", err)
	}

	// 3. Take a snapshot for consistent reads.
	snap := s.db.NewSnapshot()
	defer snap.Close()

	// 4. Evaluate the query to get matching IDs as a bitmap.
	bitmap, err := s.evaluateAST(ctx, snap, q)
	if err != nil {
		return nil, fmt.Errorf("error evaluating query: %w", err)
	}

	// 6. Apply cursor mask.
	cursor, err := options.GetCursor()
	if err != nil {
		return nil, fmt.Errorf("error decoding cursor: %w", err)
	}

	if cursor != nil {
		s.log.Info("decoded cursor", "value", *cursor)
		cursorMask := roaring64.New()
		cursorMask.AddRange(0, *cursor)
		bitmap.And(cursorMask)
	}

	// 7. Iterate in reverse order (newest first).
	it := bitmap.ReverseIterator()

	maxResults := options.GetResultsPerPage()

	nextIDs := func(max uint64) []uint64 {
		ids := []uint64{}
		for range max {
			if !it.HasNext() {
				break
			}
			ids = append(ids, it.Next())
		}
		return ids
	}

	// 8-10. Fetch payloads in batches, convert to JSON, and paginate.
	totalBytes := uint64(0)
	finished := true
	var lastID *uint64

fillLoop:
	for it.HasNext() {

		nextBatchSize := min(maxResults-uint64(len(res.Data)), 10)

		nextIDs := nextIDs(nextBatchSize)

		payloads, err := s.retrievePayloads(snap, nextIDs)
		if err != nil {
			return nil, fmt.Errorf("error retrieving payloads: %w", err)
		}

		for _, payload := range payloads {

			lastID = &payload.ID

			ed := toPayload(payload, options.GetIncludeData())
			d, err := json.Marshal(ed)
			if err != nil {
				return nil, fmt.Errorf("error marshalling entity data: %w", err)
			}
			res.Data = append(res.Data, d)
			totalBytes += uint64(len(d))

			if totalBytes > maxResultBytes {
				finished = false
				break fillLoop
			}

			if uint64(len(res.Data)) >= maxResults {
				finished = false
				break fillLoop
			}

		}

	}

	// 11. Set cursor if not finished.
	if !finished {
		res.Cursor = pointerOf(hexutil.EncodeUint64(*lastID))
	}

	return res, nil
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func pointerOf[T any](v T) *T {
	return &v
}

func filterAttributes[T any](predicate func(string) bool, m map[string]T) []Attribute[T] {
	res := []Attribute[T]{}

	for k, v := range m {
		if !predicate(k) {
			continue
		}
		res = append(res, Attribute[T]{Key: k, Value: v})
	}

	slices.SortFunc(res, func(i, j Attribute[T]) int {
		return strings.Compare(i.Key, j.Key)
	})

	return res
}

func syntheticPredicate(k string) bool {
	return strings.HasPrefix(k, "$")
}

func nonSyntheticPredicate(k string) bool {
	return !strings.HasPrefix(k, "$")
}

func anyPredicate(string) bool {
	return true
}

func toPayload(r PayloadRow, includeData IncludeData) *EntityData {
	res := &EntityData{}

	if includeData.Key {
		res.Key = pointerOf(common.BytesToHash(r.EntityKey))
	}

	if includeData.Payload {
		res.Value = r.Payload
	}

	if includeData.ContentType {
		res.ContentType = &r.ContentType
	}

	switch {
	case includeData.Attributes && includeData.SyntheticAttributes:
		res.StringAttributes = filterAttributes(anyPredicate, r.StringAttributes.Values)
		res.NumericAttributes = filterAttributes(anyPredicate, r.NumericAttributes.Values)
	case includeData.Attributes:
		res.StringAttributes = filterAttributes(nonSyntheticPredicate, r.StringAttributes.Values)
		res.NumericAttributes = filterAttributes(nonSyntheticPredicate, r.NumericAttributes.Values)
	case includeData.SyntheticAttributes:
		res.StringAttributes = filterAttributes(syntheticPredicate, r.StringAttributes.Values)
		res.NumericAttributes = filterAttributes(syntheticPredicate, r.NumericAttributes.Values)
	}

	if includeData.Expiration {
		res.ExpiresAt = pointerOf(r.NumericAttributes.Values["$expiration"])
	}

	if includeData.Creator {
		res.Creator = pointerOf(common.HexToAddress(r.StringAttributes.Values["$creator"]))
	}

	if includeData.Owner {
		res.Owner = pointerOf(common.HexToAddress(r.StringAttributes.Values["$owner"]))
	}

	if includeData.CreatedAtBlock {
		res.CreatedAtBlock = pointerOf(r.NumericAttributes.Values["$createdAtBlock"])
	}

	if includeData.LastModifiedAtBlock {
		res.LastModifiedAtBlock = pointerOf(r.NumericAttributes.Values["$lastModifiedAtBlock"])
	}

	if includeData.TransactionIndexInBlock {
		res.TransactionIndexInBlock = pointerOf(r.NumericAttributes.Values["$txIndex"])
	}

	if includeData.OperationIndexInTransaction {
		res.OperationIndexInTransaction = pointerOf(r.NumericAttributes.Values["$opIndex"])
	}

	return res
}
