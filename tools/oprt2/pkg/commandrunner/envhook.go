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

package commandrunner

import (
	"context"
	"fmt"
	"os/exec"
)

// EnvVarHook provides an easy way to set an environment variable on every
// command invokation.
type EnvVarHook struct {
	name  string
	value string
}

var _ CommandHook = &EnvVarHook{}

func NewEnvVarHook(name, value string) *EnvVarHook {
	return &EnvVarHook{}
}

func (evh *EnvVarHook) Name() string {
	return "environment variable"
}

func (evh *EnvVarHook) Command(ctx context.Context, cmd *exec.Cmd) error {
	if evh.name != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", evh.name, evh.value))
	}

	return nil
}
