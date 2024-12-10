package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
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

// CalculateBinarySizes determines the size of provided artifacts and outputs a map of
// artifacts to size in JSON like the following. The sizes emitted are in MB.
//
//	{
//	    "one": 123,
//	    "two": 456,
//	    "three": 789
//	}
func (b *Bot) CalculateBinarySizes(ctx context.Context, build string, artifacts []string, out io.Writer) error {
	stats := make(map[string]int64, len(artifacts))
	for _, artifact := range artifacts {
		if ctx.Err() != nil {
			return trace.Wrap(ctx.Err())
		}

		info, err := os.Stat(filepath.Join(build, artifact))
		if err != nil {
			return trace.Wrap(err)
		}
		stats[artifact] = info.Size()
	}

	return trace.Wrap(json.NewEncoder(out).Encode(stats))
}

// BloatCheck determines if any of the provided artifacts have increased by comparing
// the built artifacts from the current branch against the artifact sizes of the base
// branch. An error is returned if the artifacts in the current directory exceed the
// artifact size present in the base statistics by more than the errorThreshold. The
// baseStats should in form of the JSON map emitted from CalculateBinarySizes.
func (b *Bot) BloatCheck(ctx context.Context, baseStats, current string, artifacts []string, out io.Writer) error {
	var stats map[string]int64
	if err := json.Unmarshal([]byte(baseStats), &stats); err != nil {
		return trace.Wrap(err)
	}

	log.Printf("Base stats provided: %v", stats)

	skip, err := b.skipItems(ctx, skipBloatCheckPrefix)
	if err != nil {
		return trace.Wrap(err)
	}

	baseLookup := func(artifact string) (int64, error) {
		size, ok := stats[artifact]
		if !ok {
			return 0, trace.NotFound("no size provided %s found", artifact)
		}

		return size, nil
	}

	var failure bool
	output := make(map[string]result, len(artifacts))
	for _, artifact := range artifacts {
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		default:
		}

		stats, err := calculateChange(baseLookup, current, artifact)
		if err != nil {
			return err
		}

		log.Printf("artifact %s has a current size of %d", artifact, stats.currentSize)

		status := "✅"
		if slices.Contains(skip, artifact) {
			status += " skipped by admin"
		} else {
			if stats.diff > int64(warnThreshold) {
				status = "⚠️"
			}
			if stats.diff > int64(errorThreshold) {
				status = "❌"
				failure = true
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

// baseSizeFn is an abstraction that allows the base artifact
// size to be retrieved from a variety of locations.
type baseSizeFn = func(artifact string) (int64, error)

func calculateChange(base baseSizeFn, current, binary string) (stats, error) {
	baseSize, err := base(binary)
	if err != nil {
		return stats{}, trace.Wrap(err)
	}

	currentInfo, err := os.Stat(filepath.Join(current, binary))
	if err != nil {
		return stats{}, trace.Wrap(err)
	}

	// convert from bytes to MB for easier to read output
	baseMB := baseSize / (1 << 20)
	currentMB := currentInfo.Size() / (1 << 20)

	return stats{
		baseSize:    baseMB,
		currentSize: currentMB,
		diff:        currentMB - baseMB,
	}, nil
}
