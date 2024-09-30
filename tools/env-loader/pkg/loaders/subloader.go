package loaders

import "github.com/gravitational/trace"

// Loader that tries several other loaders and uses the first one that can
// successfully load provided values
type SubLoader struct {
	SubLoaders []Loader
}

func NewSubLoader(loaders ...Loader) *SubLoader {
	return &SubLoader{
		SubLoaders: loaders,
	}
}

func (sl *SubLoader) GetEnvironmentValues(bytes []byte) (map[string]string, error) {
	subloader := sl.getSubloader(bytes)
	if subloader == nil {
		return nil, trace.Errorf("found no YAML loaders for the provided content")
	}

	environmentValues, err := subloader.GetEnvironmentValues(bytes)
	if err != nil {
		return nil, trace.Wrap(err, "failed to unmarshal YAML bytes with %q YAML loader", subloader.Name())
	}

	return environmentValues, nil
}

// Gets the first subloader that can convert the provided values
func (sl *SubLoader) getSubloader(bytes []byte) Loader {
	for _, subloader := range sl.SubLoaders {
		if subloader.CanGetEnvironmentValues(bytes) {
			return subloader
		}
	}

	return nil
}

func (sl *SubLoader) CanGetEnvironmentValues(bytes []byte) bool {
	return sl.getSubloader(bytes) != nil
}
