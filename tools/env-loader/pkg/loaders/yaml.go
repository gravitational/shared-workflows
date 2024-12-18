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
	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/gravitational/trace"
	"gopkg.in/yaml.v3"
)

type plainYAMLSubloader struct{}

func (*plainYAMLSubloader) GetEnvironmentValues(yamlBytes []byte) (map[string]values.Value, error) {
	var rawEnvironmentValues map[string]string
	err := yaml.Unmarshal(yamlBytes, &rawEnvironmentValues)
	if err != nil {
		return nil, trace.Wrap(err, "failed to unmarshal YAML bytes")
	}

	// This can occur with an empty file that has a docstring.
	// Upstream yaml library bug?
	if rawEnvironmentValues == nil {
		rawEnvironmentValues = map[string]string{}
	}

	// Wrap the string in `Value` type
	environmentValues := make(map[string]values.Value, len(rawEnvironmentValues))
	for key, value := range rawEnvironmentValues {
		environmentValues[key] = values.Value{
			UnderlyingValue: value,
		}
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
	return "plain YAML"
}

type SOPSYAMLSubloader struct{}

func (*SOPSYAMLSubloader) GetEnvironmentValues(yamlBytes []byte) (map[string]values.Value, error) {
	yamlBytes, err := decrypt.Data(yamlBytes, "yaml")
	if err != nil {
		return nil, trace.Wrap(err, "failed to decrypt YAML SOPS content")
	}

	values, err := (&plainYAMLSubloader{}).GetEnvironmentValues(yamlBytes)
	if err != nil {
		return nil, trace.Wrap(err, "failed to parse decrypted YAML content")
	}

	// Mark all loaded values as secret - if they weren't, then they shouldn't
	// be encrypted. This does not currently support files with unencrypted
	// content because the SOPS library `shouldBeEncrypted` function is not
	// public.
	for key, value := range values {
		value.ShouldMask = true
		values[key] = value
	}

	return values, nil
}

func (*SOPSYAMLSubloader) CanGetEnvironmentValues(yamlBytes []byte) bool {
	// Attempt to unmarshal SOPS-specific fields to test if this is a SOPS document
	_, err := (&sopsyaml.Store{}).LoadEncryptedFile(yamlBytes)
	return err == nil
}

func (*SOPSYAMLSubloader) Name() string {
	return "SOPS YAML"
}

type YAMLLoader struct {
	*SubLoader
}

func NewYAMLLoader() *YAMLLoader {
	return &YAMLLoader{
		SubLoader: NewSubLoader(
			&SOPSYAMLSubloader{},
			&plainYAMLSubloader{},
		),
	}
}

func (*YAMLLoader) Name() string {
	return "YAML"
}
