/*
 * AMI cleanup tool
 * Copyright (C) 2024  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"context"
	"log"

	"github.com/alecthomas/kingpin/v2"

	"github.com/gravitational/shared-workflows/tools/ami-cleanup/internal"
)

var (
	Version = "0.0.0-dev"
)

func main() {
	kingpin.Version(Version)
	dryRun := kingpin.Flag("dry-run", "set to only report the expected changes without actually performing them").Bool()
	kingpin.Parse()

	ctx := context.Background()
	err := internal.NewApplicationInstance(*dryRun).Run(ctx)
	if err != nil {
		log.Fatalf("%v", err)
	}
}
