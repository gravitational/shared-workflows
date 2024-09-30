package loaders

import "github.com/gravitational/trace"

// Defines a type that can load environment values
type Loader interface {
	CanGetEnvironmentValues([]byte) bool
	GetEnvironmentValues([]byte) (map[string]string, error)
	Name() string
}

// This loader doesn't do any work itself - it just loads content via other,
// actual loaders
type subLoader struct {
	SubLoaders []Loader
}

func newSubLoader(loaders ...Loader) *subLoader {
	return &subLoader{
		SubLoaders: loaders,
	}
}

func (sl *subLoader) GetEnvironmentValues(bytes []byte) (map[string]string, error) {
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

func (sl *subLoader) getSubloader(bytes []byte) Loader {
	for _, subloader := range sl.SubLoaders {
		if subloader.CanGetEnvironmentValues(bytes) {
			return subloader
		}
	}

	return nil
}

func (sl *subLoader) CanGetEnvironmentValues(bytes []byte) bool {
	return sl.getSubloader(bytes) != nil
}

var DefaultLoader = newSubLoader(NewYamlLoader())
