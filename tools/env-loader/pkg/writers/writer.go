package writers

type Writer interface {
	FormatEnvironmentValues(values map[string]string) (string, error)
	Name() string
}

var (
	dotenvWriter  = NewDotenvWriter()
	DefaultWriter = dotenvWriter
	AllWriters    = map[string]Writer{
		dotenvWriter.Name(): dotenvWriter,
	}
)
