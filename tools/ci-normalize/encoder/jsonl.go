package encoder

import (
	"encoding/json"
	"io"
)

// JSONLEncoder encodes records as JSON Lines.
type JSONLEncoder struct {
	enc *json.Encoder
}

func NewJSONLEncoder(w io.Writer) Encoder {
	return &JSONLEncoder{
		enc: json.NewEncoder(w),
	}
}

func (e *JSONLEncoder) Encode(record any) error {
	return e.enc.Encode(record)
}
