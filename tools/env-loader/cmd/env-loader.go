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

package main

import (
	"fmt"
	"log"
	"maps"
	"os"
	"slices"

	"github.com/alecthomas/kingpin/v2"
	envloader "github.com/gravitational/shared-workflows/tools/env-loader/pkg"
	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/writers"
	"github.com/gravitational/trace"
)

const EnvVarPrefix = "ENV_LOADER_"

// This is a package-level var to assist with capturing stdout in tests
var outputPrinter = fmt.Print

type config struct {
	EnvironmentsDirectory string
	Environment           string
	ValueSets             []string
	Values                []string
	Writer                string
}

func parseCLI(args []string) *config {
	c := &config{}

	kingpin.Flag("environments-directory", "Path to the directory containing all environments, defaulting to the repo root").
		Short('d').
		Envar(EnvVarPrefix + "ENVIRONMENTS_DIRECTORY").
		StringVar(&c.EnvironmentsDirectory)

	kingpin.Flag("environment", "Name of the environment containing the values to load").
		Envar(EnvVarPrefix + "ENVIRONMENT").
		Short('e').
		Default("local").
		StringVar(&c.Environment)

	kingpin.Flag("value-set", "Name of the value set to load").
		Short('s').
		Envar(EnvVarPrefix + "VALUE_SETS").
		StringsVar(&c.ValueSets)

	kingpin.Flag("values", "Name of the specific value to output").
		Short('v').
		Envar(EnvVarPrefix + "VALUE").
		StringsVar(&c.Values)

	kingpin.Flag("format", "Output format of the value(s)").
		Short('f').
		Envar(EnvVarPrefix+"FORMAT").
		Default("dotenv").
		EnumVar(&c.Writer, slices.Collect(maps.Keys(writers.FromName))...)

	kingpin.MustParse(kingpin.CommandLine.Parse(args))
	return c
}

func getRequestedEnvValues(c *config) (map[string]string, error) {
	// Load in values
	var envValues map[string]string
	var err error
	if c.EnvironmentsDirectory != "" {
		envValues, err = envloader.LoadEnvironmentValuesInDirectory(c.EnvironmentsDirectory, c.Environment, c.ValueSets)
	} else {
		envValues, err = envloader.LoadEnvironmentValues(c.Environment, c.ValueSets)
	}

	if err != nil {
		return nil, trace.Wrap(err, "failed to load all environment values")
	}

	// Filter out values not requested
	if len(c.Values) > 0 {
		maps.DeleteFunc(envValues, func(key, _ string) bool {
			return !slices.Contains(c.Values, key)
		})
	}

	return envValues, nil
}

func run(c *config) error {
	envValues, err := getRequestedEnvValues(c)
	if err != nil {
		return trace.Wrap(err, "failed to get requested environment values")
	}

	// Build the output string
	writer := writers.FromName[c.Writer]
	envValueOutput, err := writer.FormatEnvironmentValues(envValues)
	if err != nil {
		return trace.Wrap(err, "failed to format output values with writer %q", c.Writer)
	}

	// Write it to stdout
	outputPrinter(envValueOutput)
	return nil
}

func main() {
	c := parseCLI(os.Args[1:])

	err := run(c)
	if err != nil {
		log.Fatalf("%v", err)
	}
}
