package writers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gravitational/trace"
)

// Pulled from https://hexdocs.pm/dotenvy/dotenv-file-format.html#variable-names
var keyValidationRegex = regexp.MustCompile("^[a-zA-Z_]+[a-zA-Z0-9_]*$")

// Outputs the values in .env file format
type DotenvWriter struct{}

// Create a new dotenv-format writer
func NewDotenvWriter() *DotenvWriter {
	return &DotenvWriter{}
}

func (*DotenvWriter) validateValue(key, _ string) error {
	if keyValidationRegex.MatchString(key) {
		return nil
	}

	return trace.Errorf("Environment value name %q cannot be written to a dotenv file", key)
}

func (ew *DotenvWriter) FormatEnvironmentValues(values map[string]string) (string, error) {
	lines := make([]string, 0, len(values))
	for key, value := range values {
		err := ew.validateValue(key, value)
		if err != nil {
			return "", trace.Wrap(err)
		}

		fileLine := fmt.Sprintf("%s=%s\n", key, value)
		lines = append(lines, fileLine)
	}

	return strings.Join(lines, ""), nil
}

func (*DotenvWriter) Name() string {
	return "dotenv"
}
