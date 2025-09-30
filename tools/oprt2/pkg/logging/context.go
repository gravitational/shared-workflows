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

package logging

import (
	"context"
	"log/slog"
)

// Recommended by linter check SA1029
type loggerKeyType string

const loggerKey loggerKeyType = "logger"

// DiscardLogger is a logger that discards all log data.
var DiscardLogger = slog.New(slog.DiscardHandler)

// ToCtx creates a new context with the logger.
func ToCtx(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromCtx retrieves the logger from the context. If nil, a no-op logger will be returned.
func FromCtx(ctx context.Context) *slog.Logger {
	logger := ctx.Value(loggerKey)

	if slogger, ok := logger.(*slog.Logger); ok && slogger != nil {
		return slogger
	}

	return DiscardLogger
}
