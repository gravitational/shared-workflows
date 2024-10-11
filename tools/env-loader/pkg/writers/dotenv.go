package writers

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/gravitational/trace"
)

// Pulled from https://hexdocs.pm/dotenvy/dotenv-file-format.html#variable-names
const KeyValidateionRegex = "^[a-zA-Z_]+[a-zA-Z0-9_]*$"

// Outputs the values in .env file format
type DotenvWriter struct {
	NewFileMode fs.FileMode
}

func NewDotenvWriter() *DotenvWriter {
	return &DotenvWriter{
		NewFileMode: 0600, // Default: current user can read and write, nobody else
	}
}

func (ew *DotenvWriter) FormatEnvironmentValues(values map[string]string) (string, error) {
	lines := make([]string, 0, len(values))

	keyRegex, err := regexp.Compile(KeyValidateionRegex)
	if err != nil {
		return "", trace.Wrap(err, "bug: failed to compile dotenv key validation regex")
	}

	for key, value := range values {
		if !keyRegex.MatchString(key) {
			return "", trace.Errorf("Environment value name %q cannot be written to a dotenv file", key)
		}

		fileLine := fmt.Sprintf("%s=%s\n", key, value)
		lines = append(lines, fileLine)
	}

	return strings.Join(lines, ""), nil
}

func (*DotenvWriter) Name() string {
	return "dotenv"
}
