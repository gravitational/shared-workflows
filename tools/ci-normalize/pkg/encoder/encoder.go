package encoder

type Encoder interface {
	Encode(any) error
}
