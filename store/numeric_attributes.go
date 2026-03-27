package store

import "encoding/json"

type NumericAttributes struct {
	Values map[string]uint64
}

func NewNumericAttributes(values map[string]uint64) *NumericAttributes {
	return &NumericAttributes{Values: values}
}

func (b *NumericAttributes) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.Values)
}

func (b *NumericAttributes) UnmarshalJSON(data []byte) error {
	if b.Values == nil {
		b.Values = make(map[string]uint64)
	}
	return json.Unmarshal(data, &b.Values)
}
