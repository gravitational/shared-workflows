/*
 *  Copyright 2024 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package loaders

import (
	"github.com/getsops/sops/v3/decrypt"
	sopsyaml "github.com/getsops/sops/v3/stores/yaml"
	"github.com/gravitational/trace"
	"gopkg.in/yaml.v3"
)

type plainYAMLSubloader struct{}

func (*plainYAMLSubloader) GetEnvironmentValues(yamlBytes []byte) (map[string]string, error) {
	var environmentValues map[string]string
	err := yaml.Unmarshal(yamlBytes, &environmentValues)
	if err != nil {
		return nil, trace.Wrap(err, "failed to unmarshal YAML bytes")
	}

	// This can occur with an empty file that has a docstring.
	// Upstream yaml library bug?
	if environmentValues == nil {
		environmentValues = map[string]string{}
	}

	return environmentValues, nil
}

func (pys *plainYAMLSubloader) CanGetEnvironmentValues(yamlBytes []byte) bool {
	if len(yamlBytes) == 0 {
		return false
	}

	_, err := pys.GetEnvironmentValues(yamlBytes)
	return err == nil
}

func (*plainYAMLSubloader) Name() string {
	return "plain"
}

type sopsYAMLSubloader struct{}

func (*sopsYAMLSubloader) GetEnvironmentValues(yamlBytes []byte) (map[string]string, error) {
	yamlBytes, err := decrypt.Data(yamlBytes, "yaml")
	if err != nil {
		return nil, trace.Wrap(err, "failed to decrypt YAML SOPS content")
	}

	return (&plainYAMLSubloader{}).GetEnvironmentValues(yamlBytes)
}

func (*sopsYAMLSubloader) CanGetEnvironmentValues(yamlBytes []byte) bool {
	// Attempt to unmarshal SOPS-specific fields to test if this is a SOPS document
	_, err := (&sopsyaml.Store{}).LoadEncryptedFile(yamlBytes)
	return err == nil
}

func (*sopsYAMLSubloader) Name() string {
	return "SOPS"
}

type YAMLLoader struct {
	*SubLoader
}

func NewYAMLLoader() *YAMLLoader {
	return &YAMLLoader{
		SubLoader: NewSubLoader(
			&sopsYAMLSubloader{},
			&plainYAMLSubloader{},
		),
	}
}

func (*YAMLLoader) Name() string {
	return "YAML"
}
