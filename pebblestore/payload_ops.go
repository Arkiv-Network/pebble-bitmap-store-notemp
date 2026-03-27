package pebblestore

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/cockroachdb/pebble"

	"github.com/Arkiv-Network/pebble-bitmap-store-notemp/store"
)

// UpsertPayloadParams holds the fields needed to insert or update a payload.
type UpsertPayloadParams struct {
	EntityKey         []byte
	Payload           []byte
	ContentType       string
	StringAttributes  *store.StringAttributes
	NumericAttributes *store.NumericAttributes
}

// PayloadRow represents a stored payload record with its attributes.
type PayloadRow struct {
	ID                uint64
	EntityKey         []byte
	Payload           []byte
	ContentType       string
	StringAttributes  *store.StringAttributes
	NumericAttributes *store.NumericAttributes
}

// UpsertPayload inserts or updates a payload. If an entity with the same
// entityKey already exists (via 0x03 lookup), the existing ID is reused.
// Otherwise a new ID is allocated. The reader parameter allows
// read-your-own-writes when an IndexedBatch is passed.
func (s *PebbleStore) UpsertPayload(batch *pebble.Batch, reader pebble.Reader, arg UpsertPayloadParams) (uint64, error) {
	ecKey := entityCurrentKey(arg.EntityKey)

	var id uint64

	val, closer, err := reader.Get(ecKey)
	if err == pebble.ErrNotFound {
		// No existing entity -- allocate a new ID.
		newID, allocErr := s.nextID(batch)
		if allocErr != nil {
			return 0, fmt.Errorf("pebblestore: allocate id: %w", allocErr)
		}
		id = newID
		if err := s.incrementEntityCount(batch, reader); err != nil {
			return 0, fmt.Errorf("pebblestore: increment entity count: %w", err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("pebblestore: lookup entity current: %w", err)
	} else {
		// Reuse existing ID.
		if len(val) != 8 {
			closer.Close()
			return 0, fmt.Errorf("pebblestore: entity current value has unexpected length %d", len(val))
		}
		id = binary.BigEndian.Uint64(val)
		closer.Close()
	}

	// Write entity-current pointer: entityKey -> id.
	var idBuf [8]byte
	binary.BigEndian.PutUint64(idBuf[:], id)
	if err := batch.Set(ecKey, idBuf[:], pebble.Sync); err != nil {
		return 0, fmt.Errorf("pebblestore: set entity current: %w", err)
	}

	// Encode and write the payload record.
	encoded, err := encodePayloadValue(arg.EntityKey, arg.ContentType, arg.StringAttributes, arg.NumericAttributes, arg.Payload)
	if err != nil {
		return 0, fmt.Errorf("pebblestore: encode payload: %w", err)
	}
	if err := batch.Set(payloadKey(id), encoded, pebble.Sync); err != nil {
		return 0, fmt.Errorf("pebblestore: set payload: %w", err)
	}

	return id, nil
}

// GetCurrentPayloadForEntityKey reads the entity->ID pointer (0x03), then
// fetches the payload record (0x02).
func (s *PebbleStore) GetCurrentPayloadForEntityKey(reader pebble.Reader, entityKey []byte) (PayloadRow, error) {
	ecKey := entityCurrentKey(entityKey)

	val, closer, err := reader.Get(ecKey)
	if err == pebble.ErrNotFound {
		return PayloadRow{}, fmt.Errorf("pebblestore: entity not found")
	}
	if err != nil {
		return PayloadRow{}, fmt.Errorf("pebblestore: get entity current: %w", err)
	}
	if len(val) != 8 {
		closer.Close()
		return PayloadRow{}, fmt.Errorf("pebblestore: entity current value has unexpected length %d", len(val))
	}
	id := binary.BigEndian.Uint64(val)
	closer.Close()

	pKey := payloadKey(id)
	val, closer, err = reader.Get(pKey)
	if err != nil {
		return PayloadRow{}, fmt.Errorf("pebblestore: get payload id=%d: %w", id, err)
	}
	data := make([]byte, len(val))
	copy(data, val)
	closer.Close()

	ek, contentType, strAttrs, numAttrs, payload, err := decodePayloadValue(data)
	if err != nil {
		return PayloadRow{}, fmt.Errorf("pebblestore: decode payload id=%d: %w", id, err)
	}

	return PayloadRow{
		ID:                id,
		EntityKey:         ek,
		Payload:           payload,
		ContentType:       contentType,
		StringAttributes:  strAttrs,
		NumericAttributes: numAttrs,
	}, nil
}

// DeletePayloadForEntityKey deletes both the payload record (0x02) and the
// entity pointer (0x03). It first reads the 0x03 key to discover the ID, then
// deletes both keys from the batch.
func (s *PebbleStore) DeletePayloadForEntityKey(batch *pebble.Batch, reader pebble.Reader, entityKey []byte) error {
	ecKey := entityCurrentKey(entityKey)

	val, closer, err := reader.Get(ecKey)
	if err == pebble.ErrNotFound {
		return fmt.Errorf("pebblestore: entity not found for deletion")
	}
	if err != nil {
		return fmt.Errorf("pebblestore: get entity current for deletion: %w", err)
	}
	if len(val) != 8 {
		closer.Close()
		return fmt.Errorf("pebblestore: entity current value has unexpected length %d", len(val))
	}
	id := binary.BigEndian.Uint64(val)
	closer.Close()

	if err := batch.Delete(payloadKey(id), pebble.Sync); err != nil {
		return fmt.Errorf("pebblestore: delete payload id=%d: %w", id, err)
	}
	if err := batch.Delete(ecKey, pebble.Sync); err != nil {
		return fmt.Errorf("pebblestore: delete entity current: %w", err)
	}

	if err := s.decrementEntityCount(batch, reader); err != nil {
		return fmt.Errorf("pebblestore: decrement entity count: %w", err)
	}

	return nil
}

// retrievePayloads fetches multiple payloads by their IDs. The results are
// returned sorted by ID in descending order.
func (s *PebbleStore) retrievePayloads(reader pebble.Reader, ids []uint64) ([]PayloadRow, error) {
	rows := make([]PayloadRow, 0, len(ids))

	for _, id := range ids {
		pKey := payloadKey(id)

		val, closer, err := reader.Get(pKey)
		if err == pebble.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("pebblestore: get payload id=%d: %w", id, err)
		}
		data := make([]byte, len(val))
		copy(data, val)
		closer.Close()

		ek, contentType, strAttrs, numAttrs, payload, err := decodePayloadValue(data)
		if err != nil {
			return nil, fmt.Errorf("pebblestore: decode payload id=%d: %w", id, err)
		}

		rows = append(rows, PayloadRow{
			ID:                id,
			EntityKey:         ek,
			Payload:           payload,
			ContentType:       contentType,
			StringAttributes:  strAttrs,
			NumericAttributes: numAttrs,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].ID > rows[j].ID
	})

	return rows, nil
}

// encodePayloadValue serialises a payload record into a single byte slice:
//
//	[entityKey:32][contentTypeLen:2BE][contentType][strAttrsLen:4BE][strAttrsJSON][numAttrsLen:4BE][numAttrsJSON][payload...]
func encodePayloadValue(entityKey []byte, contentType string, strAttrs *store.StringAttributes, numAttrs *store.NumericAttributes, payload []byte) ([]byte, error) {
	strJSON, err := json.Marshal(strAttrs)
	if err != nil {
		return nil, fmt.Errorf("marshal string attributes: %w", err)
	}
	numJSON, err := json.Marshal(numAttrs)
	if err != nil {
		return nil, fmt.Errorf("marshal numeric attributes: %w", err)
	}

	ctBytes := []byte(contentType)
	size := 32 + 2 + len(ctBytes) + 4 + len(strJSON) + 4 + len(numJSON) + len(payload)
	buf := make([]byte, size)

	offset := 0

	// Entity key (32 bytes).
	copy(buf[offset:], entityKey[:32])
	offset += 32

	// Content type: uint16 BE length + bytes.
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(ctBytes)))
	offset += 2
	copy(buf[offset:], ctBytes)
	offset += len(ctBytes)

	// String attributes JSON: uint32 BE length + bytes.
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(strJSON)))
	offset += 4
	copy(buf[offset:], strJSON)
	offset += len(strJSON)

	// Numeric attributes JSON: uint32 BE length + bytes.
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(numJSON)))
	offset += 4
	copy(buf[offset:], numJSON)
	offset += len(numJSON)

	// Remaining payload bytes.
	copy(buf[offset:], payload)

	return buf, nil
}

// decodePayloadValue reverses encodePayloadValue, extracting all fields from
// the binary representation.
func decodePayloadValue(data []byte) (entityKey []byte, contentType string, strAttrs *store.StringAttributes, numAttrs *store.NumericAttributes, payload []byte, err error) {
	if len(data) < 32+2 {
		return nil, "", nil, nil, nil, fmt.Errorf("payload value too short: %d bytes", len(data))
	}

	offset := 0

	// Entity key (32 bytes).
	entityKey = make([]byte, 32)
	copy(entityKey, data[offset:offset+32])
	offset += 32

	// Content type.
	if offset+2 > len(data) {
		return nil, "", nil, nil, nil, fmt.Errorf("payload value truncated at content type length")
	}
	ctLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+ctLen > len(data) {
		return nil, "", nil, nil, nil, fmt.Errorf("payload value truncated at content type")
	}
	contentType = string(data[offset : offset+ctLen])
	offset += ctLen

	// String attributes JSON.
	if offset+4 > len(data) {
		return nil, "", nil, nil, nil, fmt.Errorf("payload value truncated at string attrs length")
	}
	strLen := int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4
	if offset+strLen > len(data) {
		return nil, "", nil, nil, nil, fmt.Errorf("payload value truncated at string attrs")
	}
	strAttrs = &store.StringAttributes{}
	if err := json.Unmarshal(data[offset:offset+strLen], strAttrs); err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("unmarshal string attributes: %w", err)
	}
	offset += strLen

	// Numeric attributes JSON.
	if offset+4 > len(data) {
		return nil, "", nil, nil, nil, fmt.Errorf("payload value truncated at numeric attrs length")
	}
	numLen := int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4
	if offset+numLen > len(data) {
		return nil, "", nil, nil, nil, fmt.Errorf("payload value truncated at numeric attrs")
	}
	numAttrs = &store.NumericAttributes{}
	if err := json.Unmarshal(data[offset:offset+numLen], numAttrs); err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("unmarshal numeric attributes: %w", err)
	}
	offset += numLen

	// Remaining bytes are the payload.
	payload = make([]byte, len(data)-offset)
	copy(payload, data[offset:])

	return entityKey, contentType, strAttrs, numAttrs, payload, nil
}
