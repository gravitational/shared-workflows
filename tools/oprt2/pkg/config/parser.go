/*
 *  Copyright 2025 Gravitational, Inc
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

package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/kaptinlin/jsonschema"
)

// ParseOPRT2ConfigFile loads the config file into a new config struct and returns it.
// Validation and defaults are handled in accordance to the `jsonschema` struct tags.
func ParseOPRT2ConfigFile(configFilePath string) (*OPRT2, error) {
	configFileAsJSON, err := loadFileAsJSON(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config contents: %w", err)
	}

	if !isOPRT2ConfigValid(configFileAsJSON) {
		return nil, fmt.Errorf("config is invalid, see stderr for details")
	}

	config := &OPRT2{}
	schema := getOPRT2Schema()
	if err := schema.Unmarshal(config, configFileAsJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}

// GetOPRT2JSONSchema returns the JSON schema for the config struct.
func GetOPRT2JSONSchema() ([]byte, error) {
	schema := getOPRT2Schema()
	schemaBytes, err := json.MarshalIndent(schema, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON schema: %w", err)
	}

	return schemaBytes, nil
}

func loadFileAsJSON(configFilePath string) ([]byte, error) {
	var configFileContents []byte
	if configFilePath == "-" {
		var err error
		configFileContents, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read config from stdin: %w", err)
		}
	} else {
		var err error
		configFileContents, err = os.ReadFile(configFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file at %q: %w", configFilePath, err)
		}
	}

	configFileAsJSON, err := yaml.YAMLToJSON(configFileContents)
	if err != nil {
		return nil, fmt.Errorf("failed to convert config file to JSON: %w", err)
	}

	return configFileAsJSON, nil
}

func getOPRT2Schema() *jsonschema.Schema {
	opts := jsonschema.DefaultStructTagOptions()
	opts.AllowUntaggedFields = true
	return jsonschema.FromStructWithOptions[OPRT2](opts)
}

// isOPRT2ConfigValid validates that the provided config file contents match the JSON schema
// for [OPRT2] config. Returns true if the config is valid, false otherwise. Records error
// information to stderr.
func isOPRT2ConfigValid(configFileAsJSON []byte) bool {
	schema := getOPRT2Schema()
	if res := schema.ValidateJSON(configFileAsJSON); res != nil && !res.IsValid() {
		fmt.Fprintf(os.Stderr, "Validation of config file failed:\n")
		for field, err := range res.Errors {
			fmt.Fprintf(os.Stderr, "- %s: %s\n", field, err.Error())
		}
		fmt.Fprintln(os.Stderr)
		return false
	}

	return true
}
