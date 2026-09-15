package executor

import (
	"slices"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/manager/agent/tool"
	"github.com/stretchr/testify/require"
)

func TestStorageToolsExposeOnlyReadOnlySchema(t *testing.T) {
	count := 0
	for _, registration := range builtinRegistrations(nil) {
		d := registration.Descriptor
		if !slices.Contains(d.Protocols, tool.ProtocolS3) {
			continue
		}
		count++
		require.Equal(t, "data", d.ID.Namespace)
		require.Equal(t, "schema", d.ID.Name)
		require.Equal(t, tool.RiskReadOnly, d.Risk)
		require.Equal(t, []tool.Capability{"data.schema"}, d.Capabilities)
	}
	require.Equal(t, 1, count)
}
