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
		return nil, trace.Errorf("found no loaders for the provided content")
	}

	environmentValues, err := subloader.GetEnvironmentValues(bytes)
	if err != nil {
		return nil, trace.Wrap(err, "failed to unmarshal bytes with %q loader", subloader.Name())
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
