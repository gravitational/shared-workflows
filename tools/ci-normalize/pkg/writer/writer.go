package writer

import (
	"github.com/gravitational/trace"
)

type Writer interface {
	Write(record any) error
	Close() error
}

func New(format string, out string) (Writer, error) {
	switch format {
	case "jsonl":
		return NewJSONLWriter(out)
	default:
		return nil, trace.BadParameter("unsupported output format: %q", format)
	}
}
