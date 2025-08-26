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

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

// Tool used by `go generate` to update the YAML schema

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	schemaContents, err := config.GetOPRT2JSONSchema()
	if err != nil {
		return fmt.Errorf("failed to marshal JSON schema")
	}

	if len(os.Args) <= 1 {
		fmt.Println(string(schemaContents))
		return nil
	}

	// Save the schema
	schemaPath := os.Args[1]
	schemaDir := filepath.Dir(schemaPath)
	if err := os.MkdirAll(schemaDir, 0770); err != nil {
		return fmt.Errorf("failed to create schema directory %q: %w", schemaDir, err)
	}

	if err := os.WriteFile(schemaPath, schemaContents, 0660); err != nil {
		return fmt.Errorf("failed to write schema to %q: %w", schemaPath, err)
	}

	return nil
}
