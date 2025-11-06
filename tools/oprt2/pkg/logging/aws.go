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
	"fmt"
	"log/slog"

	awslogging "github.com/aws/smithy-go/logging"
)

// ToAWSLogger converts the provided logger into the type that the AWS SDK expects.
func ToAWSLogger(logger *slog.Logger) awslogging.Logger {
	return &awsLogger{
		slogger: logger,
	}
}

type awsLogger struct {
	slogger *slog.Logger
	ctx     context.Context
}

var _ awslogging.Logger = &awsLogger{}
var _ awslogging.ContextLogger = &awsLogger{}

func (al *awsLogger) Logf(classification awslogging.Classification, format string, v ...any) {
	switch classification {
	case awslogging.Warn:
		al.Warn(format, v...)
	case awslogging.Debug:
		al.Debug(format, v...)
	default:
		al.Warn("Attempting to log with unsupported log level %q", string(classification))
		al.Warn(format, v...)
	}
}

func (al *awsLogger) WithContext(ctx context.Context) awslogging.Logger {
	return &awsLogger{
		slogger: al.slogger,
		ctx:     ctx,
	}
}

func (al *awsLogger) Warn(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if al.ctx != nil {
		al.slogger.WarnContext(al.ctx, msg)
		return
	}
	al.slogger.Warn(msg)
}

func (al *awsLogger) Debug(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if al.ctx != nil {
		al.slogger.DebugContext(al.ctx, msg)
		return
	}
	al.slogger.Debug(msg)
}
