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
	"os/exec"
)

// Hook defines function(s) that should be called during different stages
// of a command execution's lifecycle.
type Hook interface {
	Name() string
	// Runs once, prior to executing any CLI commands.
	// If this errors, the command is never called.
	Setup(ctx context.Context) error
	// Runs once per CLI command, prior to building the command.
	// If this errors, the command is not called.
	// This can be useful for prepending arguments.
	PreCommand(ctx context.Context, name *string, args *[]string) error
	// Runs once per CLI command, prior to the command executing.
	// If this errors, the command is not called.
	Command(ctx context.Context, cmd *exec.Cmd) error
	// Called after all CLI commands have been executed, even if an error occurs.
	Cleanup(ctx context.Context) error
}
