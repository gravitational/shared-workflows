/*
Copyright 2025 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webassets

import (
	"encoding/json"
	"os"

	"github.com/gravitational/trace"
)

type Stats struct {
	BundleSizes     map[string]int64 `json:"bundleSizes"`
	BundleGzipSizes map[string]int64 `json:"bundleGzipSizes"`
	FileSizes       map[string]int64 `json:"fileSizes"`
	FileGzipSizes   map[string]int64 `json:"fileGzipSizes"`
	ModuleSizes     map[string]int64 `json:"moduleSizes"`
	ModuleGzipSizes map[string]int64 `json:"moduleGzipSizes"`
	TotalSize       int64            `json:"totalSize"`
	TotalGzipSize   int64            `json:"totalGzipSize"`
}

func LoadStats(path string) (Stats, error) {
	var report Stats

	data, err := os.ReadFile(path)
	if err != nil {
		return report, trace.Wrap(err, "failed to read stats report from %q", path)
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return report, trace.Wrap(err, "failed to unmarshal stats report from %q", path)
	}

	return report, nil
}
