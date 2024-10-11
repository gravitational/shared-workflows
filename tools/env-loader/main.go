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

const ENV_VAR_PREFIX = "ENV_LOADER_"

type config struct {
	Environment string
	ValueSet    string
	Values      []string
	Writer      string
}

func parseCli() *config {
	c := &config{}

	kingpin.Flag("environment", "Name of the environment containing the values to load").
		Envar(ENV_VAR_PREFIX + "ENVIRONMENT").
		Short('e').
		Default("local").
		StringVar(&c.Environment)

	kingpin.Flag("value-set", "Name of the value set to load").
		Short('s').
		Envar(ENV_VAR_PREFIX + "VALUE_SET").
		StringVar(&c.ValueSet)

	kingpin.Flag("values", "Name of the specific value to output").
		Short('v').
		Envar(ENV_VAR_PREFIX + "VALUE").
		StringsVar(&c.Values)

	kingpin.Flag("format", "Output format of the value(s)").
		Short('f').
		Envar(ENV_VAR_PREFIX+"FORMAT").
		Default("dotenv").
		EnumVar(&c.Writer, slices.Collect(maps.Keys(writers.AllWriters))...)

	return c
}

func run(c *config) error {
	// Load in values
	envValues, err := envloader.LoadEnvironmentValues(c.Environment, c.ValueSet)
	if err != nil {
		return trace.Wrap(err, "failed to load all environment values")
	}

	// Filter out values not requested
	if len(c.Values) > 0 {
		maps.DeleteFunc(envValues, func(key, _ string) bool {
			return !slices.Contains(c.Values, key)
		})
	}

	// Build the output string
	writer := writers.AllWriters[c.Writer]
	envValueOutput, err := writer.FormatEnvironmentValues(envValues)
	if err != nil {
		return trace.Wrap(err, "failed to format output values with writer %q", c.Writer)
	}

	// Write it to stdout
	fmt.Print(envValueOutput)
	return nil
}

func main() {
	c := parseCli()

	err := run(c)
	if err != nil {
		log.Fatalf("%v", err)
	}
}
