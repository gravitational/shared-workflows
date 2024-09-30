package writers

// Writers take environment values and create a string (usually to be written
// to a file) of a given format.
type Writer interface {
	// Take in environment key/value pairs and format them.
	FormatEnvironmentValues(values map[string]string) (string, error)
	// Human-readable name of the writer, usually the output format.
	Name() string
}

var (
	dotenvWriter  = NewDotenvWriter()
	DefaultWriter = dotenvWriter

	// A map of all writers available.
	AllWriters = map[string]Writer{
		dotenvWriter.Name(): dotenvWriter,
	}
)
