package queries

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSingleSelectOptionID(t *testing.T) {
	status := singleSelectField("PVTSSF_status", "Status", []SingleSelectFieldOptions{
		{ID: "opt_todo", Name: "Todo"},
		{ID: "opt_inprog", Name: "In Progress"},
		{ID: "opt_done", Name: "Done"},
	})

	t.Run("resolves an option name to its id", func(t *testing.T) {
		id, err := ResolveSingleSelectOptionID(status, "In Progress")
		require.NoError(t, err)
		assert.Equal(t, "opt_inprog", id)
	})

	t.Run("resolves an option name case-insensitively", func(t *testing.T) {
		id, err := ResolveSingleSelectOptionID(status, "in progress")
		require.NoError(t, err)
		assert.Equal(t, "opt_inprog", id)
	})

	t.Run("case-insensitive match that hits multiple options is ambiguous", func(t *testing.T) {
		dup := singleSelectField("PVTSSF_status", "Status", []SingleSelectFieldOptions{
			{ID: "opt_lower", Name: "done"},
			{ID: "opt_upper", Name: "Done"},
		})
		_, err := ResolveSingleSelectOptionID(dup, "DONE")
		require.Error(t, err)
		var amb *OptionAmbiguousError
		require.True(t, errors.As(err, &amb))
		assert.ElementsMatch(t, []string{"opt_lower", "opt_upper"}, amb.Candidates)
	})

	t.Run("not found lists candidate option names", func(t *testing.T) {
		_, err := ResolveSingleSelectOptionID(status, "In progres")
		require.Error(t, err)
		var nf *OptionNotFoundError
		require.True(t, errors.As(err, &nf))
		assert.Equal(t, "Status", nf.FieldName)
		assert.Equal(t, []string{"Todo", "In Progress", "Done"}, nf.Candidates)
		assert.Contains(t, err.Error(), "available options: Todo, In Progress, Done")
	})

	t.Run("ambiguous option lists candidate ids", func(t *testing.T) {
		dup := singleSelectField("PVTSSF_status", "Status", []SingleSelectFieldOptions{
			{ID: "opt_a", Name: "Done"},
			{ID: "opt_b", Name: "Done"},
		})
		_, err := ResolveSingleSelectOptionID(dup, "Done")
		require.Error(t, err)
		var amb *OptionAmbiguousError
		require.True(t, errors.As(err, &amb))
		assert.ElementsMatch(t, []string{"opt_a", "opt_b"}, amb.Candidates)
	})

	t.Run("wrong field type is rejected", func(t *testing.T) {
		_, err := ResolveSingleSelectOptionID(textField("PVTF_title", "Title"), "In Progress")
		require.Error(t, err)
		var wt *WrongFieldTypeError
		require.True(t, errors.As(err, &wt))
		assert.Equal(t, "TEXT", wt.DataType)
		assert.Equal(t, "SINGLE_SELECT", wt.Expected)
	})
}

func TestProjectItemIDByURL(t *testing.T) {
	client := NewTestClient()
	_, err := client.ProjectItemIDByURL("https://github.com/monalisa/repo/issues/1", "PVT_target", 5)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
