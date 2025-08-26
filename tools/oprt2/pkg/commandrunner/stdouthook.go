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
	"bytes"
	"context"
	"io"
	"os/exec"
)

// StdoutHook provides an easy way to get stdout of every command invokation.
type StdoutHook struct {
	buffer []byte
}

var _ CommandHook = &StdoutHook{}

func NewStdoutHook(buffer []byte) *StdoutHook {
	return &StdoutHook{
		buffer: buffer,
	}
}

func (soh *StdoutHook) Name() string {
	return "stdout"
}

func (soh *StdoutHook) Command(ctx context.Context, cmd *exec.Cmd) error {
	if soh.buffer == nil {
		cmd.Stdout = io.Discard
	} else {
		// Reset the length to 0, but don't release the allocated space
		soh.buffer = soh.buffer[:0]
		cmd.Stdout = bytes.NewBuffer(soh.buffer)
	}

	return nil
}
