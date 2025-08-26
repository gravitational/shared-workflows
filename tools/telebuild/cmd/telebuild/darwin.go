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
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
)

// DarwinCmd is a kong struct that contains flags and methods for building darwin artifacts.
// It is meant to be used as a subcommand of the Telebuild CLI.
type DarwinCmd struct {
	Tarball DarwinTarball `cmd:"" help:"Build a tarball containing darwin binaries"`

	DarwinCommonFlags
}

// DarwinCommonFlags is a kong struct that contains flags common to all darwin commands.
// This can be embedded in other kong structs to avoid duplication of common flags.
type DarwinCommonFlags struct {
	// Retry is the number of times to retry notarization in case of failure.
	Retry int `group:"Notarization Optional Flags" help:"Retry notarization in case of failure."`

	// KeychainProfile is the keychain profile to use for notarization.
	KeychainProfile string `group:"Notarization Required Flags" env:"${envPrefix}KEYCHAIN_PROFILE" help:"Keychain profile to use for notarization. Use \"man notarytool\" for authentication options."`
	// SigningID is the signing identity to use for codesigning.
	SigningID string `group:"Notarization Required Flags" env:"${envPrefix}SIGNING_ID" help:"Signing Identity to use for codesigning."`
	// TeamID is the Apple Developer Team ID.
	TeamID string `group:"Notarization Required Flags" env:"${envPrefix}TEAM_ID" help:"Team ID is the unique identifier for the Apple Developer account."`

	// CI detects whether the build is running in a CI environment.
	// This is a common flag
	CI bool `hidden:"" env:"CI" help:"CI mode. Disables dry-run."`
}

var (
	darwinSupportedTarballArchs = []string{ArchAMD64, ArchARM64, ArchUniversal}

	darwinKongVars = kong.Vars{
		"darwinSupportedTarballArchs": strings.Join(darwinSupportedTarballArchs, ","),
	}
)

// DarwinTarball is a kong struct that contains flags and methods for building darwin tarballs.
type DarwinTarball struct {
	// Arch is the architecture to build the tarball for.
	Arch string `help:"Supported architectures ${darwinSupportedTarballArchs}" enum:"${darwinSupportedTarballArchs}" required:""`
}

// Run executes the tarball build process.
func (cmd *DarwinTarball) Run(cli *CLI, parent *DarwinCmd) error {
	switch cmd.Arch {
	case ArchUniversal:
		return cmd.buildDarwinUniversalTarball(cli)
	default:
		return fmt.Errorf("unsupported architecture for darwin: %s", cmd.Arch)
	}
}

func (cmd *DarwinTarball) buildDarwinUniversalTarball(cli *CLI) error {
	return errors.New("TODO")
}
