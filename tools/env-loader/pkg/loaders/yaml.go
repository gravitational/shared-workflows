package loaders

import (
	"github.com/getsops/sops/v3/decrypt"
	sopsYaml "github.com/getsops/sops/v3/stores/yaml"
	"github.com/gravitational/trace"
	"gopkg.in/yaml.v3"
)

type plainYamlSubloader struct{}

func (*plainYamlSubloader) GetEnvironmentValues(yamlBytes []byte) (map[string]string, error) {
	var environmentValues map[string]string
	err := yaml.Unmarshal(yamlBytes, &environmentValues)
	if err != nil {
		return nil, trace.Wrap(err, "failed to unmarshal YAML bytes")
	}

	return environmentValues, nil
}

func (pys *plainYamlSubloader) CanGetEnvironmentValues(yamlBytes []byte) bool {
	_, err := pys.GetEnvironmentValues(yamlBytes)
	return err == nil
}

func (*plainYamlSubloader) Name() string {
	return "plain"
}

type sopsYamlSubloader struct{}

func (*sopsYamlSubloader) GetEnvironmentValues(yamlBytes []byte) (map[string]string, error) {
	yamlBytes, err := decrypt.Data(yamlBytes, "yaml")
	if err != nil {
		return nil, trace.Wrap(err, "failed to decrypt YAML SOPS content")
	}

	return (&plainYamlSubloader{}).GetEnvironmentValues(yamlBytes)
}

func (*sopsYamlSubloader) CanGetEnvironmentValues(yamlBytes []byte) bool {
	// Attempt to unmarshal SOPS-specific fields to test if this is a SOPS document
	_, err := (&sopsYaml.Store{}).LoadEncryptedFile(yamlBytes)
	return err == nil
}

func (*sopsYamlSubloader) Name() string {
	return "SOPS"
}

type YamlLoader struct {
	*subLoader
	// subLoaders []Loader
}

func NewYamlLoader() *YamlLoader {
	return &YamlLoader{
		subLoader: newSubLoader(
			&sopsYamlSubloader{},
			&plainYamlSubloader{},
		),
	}
}

func (*YamlLoader) Name() string {
	return "YAML"
}
