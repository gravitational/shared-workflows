package encoder

// Encoder converts records into bytes written to an io.Writer.
type Encoder interface {
	Encode(any) error
}
