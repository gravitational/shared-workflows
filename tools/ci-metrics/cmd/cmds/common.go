// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmds

import (
	kingpin "github.com/alecthomas/kingpin/v2"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
)

// registerAthenaConfigFlags creates the CLI interface to configure [athena.Config].
func registerAthenaConfigFlags(cmd *kingpin.CmdClause, cfg *athena.Config) {
	cmd.Flag("database", "Database holding both tables").
		Required().
		StringVar(&cfg.Database)

	cmd.Flag("workgroup", "Athena workgroup").
		Default("primary").
		StringVar(&cfg.Workgroup)

	cmd.Flag("region", "AWS region; defaults to the ambient credential chain").
		StringVar(&cfg.Region)

	cmd.Flag("results", "S3 prefix for Athena query results; unset defers to the workgroup setting").
		StringVar(&cfg.OutputLocation)
}
