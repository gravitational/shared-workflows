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
	"slices"

	"github.com/alecthomas/kingpin/v2"
	envloader "github.com/gravitational/shared-workflows/tools/env-loader/pkg"
	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/writers"
	"github.com/gravitational/trace"
)

const EnvVarPrefix = "ENV_LOADER_"

type config struct {
	Environment string
	ValueSets   []string
	Values      []string
	Writer      string
}

func parseCLI() *config {
	c := &config{}

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

	kingpin.Parse()

	return c
}

func run(c *config) error {
	// Load in values
	envValues, err := envloader.LoadEnvironmentValues(c.Environment, c.ValueSets)
	if err != nil {
		return trace.Wrap(err, "failed to load all environment values")
	}

	// Filter out values not requested
	maps.DeleteFunc(envValues, func(key, _ string) bool {
		return !slices.Contains(c.Values, key)
	})

	// Build the output string
	writer := writers.FromName[c.Writer]
	envValueOutput, err := writer.FormatEnvironmentValues(map[string]string{})
	if err != nil {
		return trace.Wrap(err, "failed to format output values with writer %q", c.Writer)
	}

	// Write it to stdout
	fmt.Print(envValueOutput)
	return nil
}

func main() {
	c := parseCLI()

	err := run(c)
	if err != nil {
		log.Fatalf("%v", err)
	}
}
