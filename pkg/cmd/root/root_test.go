package root

import (
	"testing"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNewCmdRoot_AllowsUnauthenticatedCommands(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")

	ios, _, _, stderr := iostreams.Test()
	cfg, _ := config.NewIsolatedTestConfig(t, "")
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (gh.Config, error) { return cfg, nil },
		Browser:   &browser.Stub{},
		ExtensionManager: &extensions.ExtensionManagerMock{
			ListFunc: func() []extensions.Extension { return nil },
		},
	}

	rootCmd, err := NewCmdRoot(f, &telemetry.NoOpService{}, "N/A", "")
	require.NoError(t, err)

	var ran bool
	testCmd := &cobra.Command{
		Use: "unauth-test",
		RunE: func(cmd *cobra.Command, args []string) error {
			ran = true
			return nil
		},
	}
	rootCmd.AddCommand(testCmd)
	require.True(t, cmdutil.IsAuthCheckEnabled(testCmd))
	require.False(t, cmdutil.CheckAuth(cfg))

	rootCmd.SetArgs([]string{"unauth-test"})
	err = rootCmd.Execute()
	require.NoError(t, err)
	require.True(t, ran)
	require.Empty(t, stderr.String())
}
