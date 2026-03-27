package store

import "encoding/json"

type StringAttributes struct {
	Values map[string]string
}

func NewStringAttributes(values map[string]string) *StringAttributes {
	return &StringAttributes{Values: values}
}

func (b *StringAttributes) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.Values)
}

func (b *StringAttributes) UnmarshalJSON(data []byte) error {
	if b.Values == nil {
		b.Values = make(map[string]string)
	}
	return json.Unmarshal(data, &b.Values)
}
