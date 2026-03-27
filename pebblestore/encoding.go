package pebblestore

import "encoding/binary"

const (
	prefixLastBlock     byte = 0x01
	prefixPayload       byte = 0x02
	prefixEntityCurrent byte = 0x03
	prefixIDCounter     byte = 0x06
	prefixEntityCount   byte = 0x07
	prefixStringBitmap  byte = 0x10
	prefixNumericBitmap byte = 0x20
)

// lastBlockKey returns the single key used to store the last processed block number.
func lastBlockKey() []byte {
	return []byte{prefixLastBlock}
}

// payloadKey returns the key for a payload record: prefix + 8-byte big-endian ID.
func payloadKey(id uint64) []byte {
	key := make([]byte, 9)
	key[0] = prefixPayload
	binary.BigEndian.PutUint64(key[1:], id)
	return key
}

// entityCurrentKey returns the key for an entity's current pointer.
// entityKey must be exactly 32 bytes.
func entityCurrentKey(entityKey []byte) []byte {
	key := make([]byte, 1+32)
	key[0] = prefixEntityCurrent
	copy(key[1:], entityKey[:32])
	return key
}

// idCounterKey returns the single key used to store the next ID counter.
func idCounterKey() []byte {
	return []byte{prefixIDCounter}
}

// entityCountKey returns the single key used to store the entity count.
func entityCountKey() []byte {
	return []byte{prefixEntityCount}
}

// stringBitmapKey returns the key for a string bitmap entry:
// prefix + 2-byte name length + name + 2-byte value length + value.
func stringBitmapKey(name, value string) []byte {
	nameBytes := []byte(name)
	valueBytes := []byte(value)
	key := make([]byte, 1+2+len(nameBytes)+2+len(valueBytes))
	key[0] = prefixStringBitmap
	binary.BigEndian.PutUint16(key[1:], uint16(len(nameBytes)))
	copy(key[3:], nameBytes)
	offset := 3 + len(nameBytes)
	binary.BigEndian.PutUint16(key[offset:], uint16(len(valueBytes)))
	copy(key[offset+2:], valueBytes)
	return key
}

// numericBitmapKey returns the key for a numeric bitmap entry:
// prefix + 2-byte name length + name + 8-byte big-endian value.
func numericBitmapKey(name string, value uint64) []byte {
	nameBytes := []byte(name)
	key := make([]byte, 1+2+len(nameBytes)+8)
	key[0] = prefixNumericBitmap
	binary.BigEndian.PutUint16(key[1:], uint16(len(nameBytes)))
	copy(key[3:], nameBytes)
	binary.BigEndian.PutUint64(key[3+len(nameBytes):], value)
	return key
}

// stringBitmapNamePrefix returns the key prefix for iterating all string bitmap
// entries for a given attribute name.
func stringBitmapNamePrefix(name string) []byte {
	nameBytes := []byte(name)
	prefix := make([]byte, 1+2+len(nameBytes))
	prefix[0] = prefixStringBitmap
	binary.BigEndian.PutUint16(prefix[1:], uint16(len(nameBytes)))
	copy(prefix[3:], nameBytes)
	return prefix
}

// numericBitmapNamePrefix returns the key prefix for iterating all numeric bitmap
// entries for a given attribute name.
func numericBitmapNamePrefix(name string) []byte {
	nameBytes := []byte(name)
	prefix := make([]byte, 1+2+len(nameBytes))
	prefix[0] = prefixNumericBitmap
	binary.BigEndian.PutUint16(prefix[1:], uint16(len(nameBytes)))
	copy(prefix[3:], nameBytes)
	return prefix
}

// decodeStringBitmapKey extracts the name and value from a string bitmap key.
func decodeStringBitmapKey(key []byte) (name, value string) {
	// key[0] is the prefix byte
	nameLen := binary.BigEndian.Uint16(key[1:3])
	name = string(key[3 : 3+nameLen])
	valueOffset := 3 + nameLen
	valueLen := binary.BigEndian.Uint16(key[valueOffset : valueOffset+2])
	value = string(key[valueOffset+2 : valueOffset+2+valueLen])
	return name, value
}

// decodeNumericBitmapKey extracts the name and numeric value from a numeric bitmap key.
func decodeNumericBitmapKey(key []byte) (name string, value uint64) {
	// key[0] is the prefix byte
	nameLen := binary.BigEndian.Uint16(key[1:3])
	name = string(key[3 : 3+nameLen])
	value = binary.BigEndian.Uint64(key[3+nameLen:])
	return name, value
}

// prefixUpperBound returns the immediate successor of prefix for use as an
// exclusive upper bound in range scans. It increments the last byte, propagating
// the carry. Returns nil if the prefix is all 0xFF bytes (meaning the scan should
// be unbounded).
func prefixUpperBound(prefix []byte) []byte {
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	for i := len(upper) - 1; i >= 0; i-- {
		upper[i]++
		if upper[i] != 0 {
			return upper
		}
	}
	return nil
}
