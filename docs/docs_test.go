package docs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSwaggerJSON_InjectsHostAndScheme(t *testing.T) {
	out, err := GetSwaggerJSON("api.example.com", "https")
	require.NoError(t, err)

	var spec map[string]any
	require.NoError(t, json.Unmarshal(out, &spec))
	assert.Equal(t, "api.example.com", spec["host"])
	assert.Equal(t, []any{"https"}, spec["schemes"])
	assert.Equal(t, "2.0", spec["swagger"])
	assert.Contains(t, spec["paths"], "/api/posts/ai-generate")
}

func TestGetSwaggerJSON_InvalidEmbeddedSpec(t *testing.T) {
	original := specJSON
	t.Cleanup(func() { specJSON = original })
	specJSON = []byte("{broken")

	out, err := GetSwaggerJSON("h", "http")

	assert.Nil(t, out)
	assert.Error(t, err)
}
