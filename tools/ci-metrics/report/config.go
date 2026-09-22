// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package report

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/gravitational/trace"
	"go.yaml.in/yaml/v3"
)

const (
	// EnvConfigBody is the name of an env var that contains the body of the config file.
	EnvConfigBody = "CI_METRICS_REPORT_CONFIG"
	// EnvConfigPath  is the name of an env var that contains the path to the conifg file.
	EnvConfigPath = "CI_METRICS_REPORT_CONFIG_FILE"
)

// defaultWindowDays is the window used when config and flags both leave it
// unset.
const defaultWindowDays = 14

// Config declares which reports to run and where to send them.
type Config struct {
	// Tables names the tables to read
	Tables Tables `yaml:"tables"`
	// Window defines the reporting window
	Window WindowConfig `yaml:"window"`
	// Reporters are destinations, keyed by a name that
	// [ReportConfig.Reporters] refers to. Naming instances rather than types
	// is what lets two Slack channels, or two row limits, coexist.
	Reporters map[string]ReporterConfig `yaml:"reporters"`
	// Reports are the configurations for reports keyed by their name.
	Reports map[string]ReportConfig `yaml:"reports"`
}

// WindowConfig defines the reporting window
type WindowConfig struct {
	// Days counts back from today, inclusive.
	Days int `yaml:"days"`
}

// ReporterConfig describes one [reporter.Reporter].
type ReporterConfig struct {
	// Type names the kind of destination, such as "stdout".
	Type string `yaml:"type"`
	// MaxRows caps rows rendered per table.
	MaxRows int `yaml:"max_rows"`
}

// ReportConfig tunes one report and names its destinations.
type ReportConfig struct {
	// Reporters names entries in [Config.Reporters].
	Reporters []string `yaml:"reporters"`
	// Reporter specific params. Each can be different.
	Params yaml.Node `yaml:"params"`
}

// DefaultConfig makes both flaky reports available, to stdout.
func DefaultConfig() *Config {
	return &Config{
		Reporters: map[string]ReporterConfig{
			"stdout": {Type: "stdout"},
		},
		Reports: map[string]ReportConfig{
			FlakyRollupName: {},
			FlakyDailyName:  {},
		},
	}
}

// ReportNames returns the configured report names, sorted.
func (c *Config) ReportNames() []string {
	out := make([]string, 0, len(c.Reports))
	for name := range c.Reports {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// checkAndSetDefaults validates the config and fills in what it can.
func (c *Config) checkAndSetDefaults() error {
	if err := c.Tables.checkAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}

	if c.Window.Days < 0 {
		return trace.BadParameter("window days must not be negative, got %d", c.Window.Days)
	}
	if c.Window.Days == 0 {
		c.Window.Days = defaultWindowDays
	}

	if len(c.Reporters) == 0 {
		c.Reporters = map[string]ReporterConfig{"stdout": {Type: "stdout"}}
	}
	for name, r := range c.Reporters {
		if name == "" {
			return trace.BadParameter("reporter has an empty name")
		}
		if r.Type == "" {
			return trace.BadParameter("reporter %s has no type", name)
		}
		if r.MaxRows < 0 {
			return trace.BadParameter("reporter %s has negative max_rows", name)
		}
	}

	if len(c.Reports) == 0 {
		return trace.BadParameter("no reports configured, want one or more of: %s",
			strings.Join(Names(), ", "))
	}

	for _, name := range c.ReportNames() {
		if _, ok := Get(name); !ok {
			return trace.BadParameter("unknown report %q, want one of: %s",
				name, strings.Join(Names(), ", "))
		}

		r := c.Reports[name]

		// An empty list means every reporter, which is the sensible reading of
		// "I configured one destination and did not repeat myself".
		if len(r.Reporters) == 0 {
			for reporter := range c.Reporters {
				r.Reporters = append(r.Reporters, reporter)
			}
			slices.Sort(r.Reporters)
			c.Reports[name] = r
			continue
		}

		for _, reporter := range r.Reporters {
			if _, ok := c.Reporters[reporter]; !ok {
				return trace.BadParameter(
					"report %s refers to undefined reporter %q", name, reporter)
			}
		}
	}

	return nil
}

// DecodeParams decodes a report's params into its registered parameter type.
func (r ReportConfig) DecodeParams(name string, def Definition) (any, error) {
	params := def.NewParams()
	if r.Params.IsZero() {
		return params, nil
	}

	raw, err := yaml.Marshal(&r.Params)
	if err != nil {
		return nil, trace.Wrap(err, "re-encoding %s params", name)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(params); err != nil && !errors.Is(err, io.EOF) {
		return nil, trace.Wrap(err, "decoding %s params", name)
	}

	return params, nil
}

// open resolves the config source. It returns a nil reader and a nil error
// when no source was supplied at all, which is distinct from a supplied source
// that could not be read.
func open(flagPath string) (io.ReadCloser, error) {
	switch {
	case flagPath == "-":
		return io.NopCloser(os.Stdin), nil
	case flagPath != "":
		f, err := os.Open(flagPath)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return f, nil
	}

	// EnvConfigPath is handled by the CLI already and will arrive as flagPath, so only fallback
	// to see if the body of the config is passed as an env var.
	if body, ok := os.LookupEnv(EnvConfigBody); ok {
		return io.NopCloser(strings.NewReader(body)), nil
	}

	return nil, nil
}

// LoadConfig reads and validates the config. With no config from any source it
// falls back to [DefaultConfig].
func LoadConfig(flagPath string) (*Config, error) {
	r, err := open(flagPath)
	if err != nil {
		return nil, trace.Wrap(err, "reading config")
	}

	cfg := &Config{}
	if r == nil {
		cfg = DefaultConfig()
	} else {
		defer func() { _ = r.Close() }()

		dec := yaml.NewDecoder(r)
		dec.KnownFields(true)
		if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
			return nil, trace.Wrap(err, "parsing config")
		}
	}

	if err := cfg.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err, "validating config")
	}

	return cfg, nil
}
