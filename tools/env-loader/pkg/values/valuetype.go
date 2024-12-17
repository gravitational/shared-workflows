package values

type Value struct {
	UnderlyingValue string
	ShouldMask      bool
}

func (v *Value) String() string {
	return v.UnderlyingValue
}
