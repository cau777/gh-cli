package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotSupportedError(t *testing.T) {
	err := NotSupportedError("pr revert requires GraphQL")
	require.ErrorIs(t, err, ErrNotSupported)
	assert.Equal(t, "not all features are supported: pr revert requires GraphQL", err.Error())

	bare := NotSupportedError("")
	require.ErrorIs(t, bare, ErrNotSupported)
	assert.Equal(t, "not all features are supported", bare.Error())
}

func TestValidateSupportedFields(t *testing.T) {
	unsupported := map[string]string{
		"projectItems":   "Projects V2 has no REST equivalent",
		"reviewDecision": "computed reviewDecision requires GraphQL",
	}

	require.NoError(t, ValidateSupportedFields([]string{"number", "title"}, unsupported))
	require.NoError(t, ValidateSupportedFields(nil, unsupported))

	err := ValidateSupportedFields([]string{"number", "projectItems", "reviewDecision"}, unsupported)
	require.ErrorIs(t, err, ErrNotSupported)
	assert.Contains(t, err.Error(), "projectItems")
	assert.Contains(t, err.Error(), "reviewDecision")
}
