package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gravitational/trace"
)

const (
	// skipBloatCheckPrefix the comment prefix to use in order to
	// skip particular artifacts from the bloat check.
	skipBloatCheckPrefix = "/excludebloat"
	// warnThreshold is the amount of MB that a binary may increase and
	// only a warning is logged
	warnThreshold = 1
	// errorThreshold is the amount of MB that a binary cannot exceed without
	// failing the check.
	errorThreshold = 3
)

// BloatCheck determines if any of the provided artifacts have increased. An error
// is returned if the artifacts in the current directory exceed the same artifact in
// the base directory by more than the errorThreshold.
func (b *Bot) BloatCheck(ctx context.Context, base, current string, artifacts []string, out io.Writer) error {
	output := make(map[string]result, len(artifacts))

	skip, err := b.skipItems(ctx, skipBloatCheckPrefix)
	if err != nil {
		return trace.Wrap(err)
	}

	var failure bool
	for _, artifact := range artifacts {
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		default:
		}

		skipped := false
		for _, s := range skip {
			if s == artifact {
				skipped = true
				break
			}
		}

		stats, err := calculateChange(base, current, artifact)
		if err != nil {
			return err
		}

		status := "✅"
		if skipped {
			status += " skipped by admin"
		} else {
			if stats.diff > int64(warnThreshold) {
				status = "⚠️"
			}
			if stats.diff > int64(errorThreshold) {
				status = "❌"
				failure = !skipped
			}
		}

		output[artifact] = result{
			baseSize:    fmt.Sprintf("%dMB", stats.baseSize),
			currentSize: fmt.Sprintf("%dMB", stats.currentSize),
			change:      fmt.Sprintf("%dMB %s", stats.diff, status),
		}
	}

	if err := renderMarkdownTable(out, output); err != nil {
		return err
	}

	if failure {
		return errors.New("binary bloat detected - at least one binary increased by more than the allowed threshold")
	}

	return nil
}

type result struct {
	baseSize    string
	currentSize string
	change      string
}

func renderMarkdownTable(w io.Writer, data map[string]result) error {
	titles := []string{"Binary", "Base Size", "Current Size", "Change"}

	// get the initial padding from the titles
	padding := map[string]int{}
	for _, v := range titles {
		padding[v] = len(v)
	}

	max := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}

	// get the largest item from the title or items in the column to determine
	// the actual padding
	for k, column := range data {
		padding["Binary"] = max(padding["Binary"], len(k))
		padding["Base Size"] = max(padding["Base Size"], len(column.baseSize))
		padding["Current Size"] = max(padding["Current Size"], len(column.currentSize))
		padding["Change"] = max(padding["Change"], utf8.RuneCountInString(column.change))
	}

	format := strings.Repeat("| %%-%ds ", len(padding)) + "|\n"
	paddings := []interface{}{
		padding["Binary"],
		padding["Base Size"],
		padding["Current Size"],
		padding["Change"],
	}
	format = fmt.Sprintf(format, paddings...)

	// write the heading and title
	buf := bytes.NewBufferString("# Bloat Check Results\n")
	row := []any{"Binary", "Base Size", "Current Size", "Change"}
	buf.WriteString(fmt.Sprintf(format, row...))

	// write the delimiter
	row = []interface{}{"", "", "", ""}
	buf.WriteString(strings.Replace(fmt.Sprintf(format, row...), " ", "-", -1))

	// write the rows
	for k, column := range data {
		row := []interface{}{k, column.baseSize, column.currentSize, column.change}
		buf.WriteString(fmt.Sprintf(format, row...))
	}

	_, err := w.Write(buf.Bytes())
	return trace.Wrap(err)
}

type stats struct {
	baseSize    int64
	currentSize int64
	diff        int64
}

func calculateChange(base, current, binary string) (stats, error) {
	baseInfo, err := os.Stat(filepath.Join(base, binary))
	if err != nil {
		return stats{}, trace.Wrap(err)
	}

	currentInfo, err := os.Stat(filepath.Join(current, binary))
	if err != nil {
		return stats{}, trace.Wrap(err)
	}

	// convert from bytes to MB for easier to read output
	baseMB := baseInfo.Size() / (1 << 20)
	currentMB := currentInfo.Size() / (1 << 20)

	return stats{
		baseSize:    baseMB,
		currentSize: currentMB,
		diff:        currentMB - baseMB,
	}, nil
}
