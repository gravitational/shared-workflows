package loaders

// Defines a type that can load environment values from raw byte sources
type Loader interface {
	// Returns true if the byte array can be converted into a key/value map
	CanGetEnvironmentValues([]byte) bool
	// Converts the byte array into a key/value map
	GetEnvironmentValues([]byte) (map[string]string, error)
	// Human-readable name of the loader
	Name() string
}

// Generic loader that can load values of any supported format
var DefaultLoader = NewSubLoader(NewYamlLoader())
