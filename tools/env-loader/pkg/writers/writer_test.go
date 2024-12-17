package writers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Tests that apply to all writer implementations

func TestWriterName(t *testing.T) {
	for listedName, writer := range FromName {
		require.NotEmpty(t, listedName, "writer name for %v is empty", writer)
		require.Equal(t, listedName, writer.Name(), "writer name for %v is not consistent", writer)
	}
}
