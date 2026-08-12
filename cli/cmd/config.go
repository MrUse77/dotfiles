package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

const (
	authorizeDriftFlag = "authorize-drift"
	themeReplaceFlag   = "theme-replace"
	offlineFlag        = "offline"
	jsonFlag           = "json"
)

type configApplyRequest struct {
	Tag            string
	AuthorizeDrift string
	ThemeReplace   string
}

type configRollbackRequest struct {
	AuthorizeDrift string
	ThemeReplace   string
	Offline        bool
}

type configStatusRequest struct {
	JSON bool
}

type configOperations interface {
	Apply(context.Context, io.Writer, configApplyRequest) error
	Rollback(context.Context, io.Writer, configRollbackRequest) error
	Status(context.Context, io.Writer, configStatusRequest) error
}

type configOperationsFactory func(*cobra.Command) configOperations

func newConfigCommand(newOperations configOperationsFactory) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Manage versioned MoonArch configuration releases",
		Args:  cobra.NoArgs,
	}
	command.AddCommand(
		newConfigApplyCommand(newOperations),
		newConfigRollbackCommand(newOperations),
		newConfigStatusCommand(newOperations),
	)
	return command
}

func newConfigApplyCommand(newOperations configOperationsFactory) *cobra.Command {
	command := &cobra.Command{
		Use:   "apply config-vX.Y.Z",
		Short: "Apply one exact configuration release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identity, err := release.ParseConfigVersion(args[0])
			if err != nil {
				return err
			}
			operations, err := configOperationsFor(cmd, newOperations)
			if err != nil {
				return err
			}
			authorizeDrift, err := cmd.Flags().GetString(authorizeDriftFlag)
			if err != nil {
				return err
			}
			themeReplace, err := cmd.Flags().GetString(themeReplaceFlag)
			if err != nil {
				return err
			}
			return operations.Apply(cmd.Context(), cmd.OutOrStdout(), configApplyRequest{
				Tag:            identity.Tag,
				AuthorizeDrift: authorizeDrift,
				ThemeReplace:   themeReplace,
			})
		},
	}
	command.Flags().String(authorizeDriftFlag, "", "Authorize the exact drift evidence token printed by a prior preflight")
	command.Flags().String(themeReplaceFlag, "", "Replace an unavailable or invalid current theme with this desired bundle")
	return command
}

func newConfigRollbackCommand(newOperations configOperationsFactory) *cobra.Command {
	command := &cobra.Command{
		Use:   "rollback",
		Short: "Reapply the retained previous configuration release offline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			operations, err := configOperationsFor(cmd, newOperations)
			if err != nil {
				return err
			}
			authorizeDrift, err := cmd.Flags().GetString(authorizeDriftFlag)
			if err != nil {
				return err
			}
			themeReplace, err := cmd.Flags().GetString(themeReplaceFlag)
			if err != nil {
				return err
			}
			offline, err := cmd.Flags().GetBool(offlineFlag)
			if err != nil {
				return err
			}
			return operations.Rollback(cmd.Context(), cmd.OutOrStdout(), configRollbackRequest{
				AuthorizeDrift: authorizeDrift,
				ThemeReplace:   themeReplace,
				Offline:        offline,
			})
		},
	}
	command.Flags().String(authorizeDriftFlag, "", "Authorize the exact drift evidence token printed by a prior preflight")
	command.Flags().String(themeReplaceFlag, "", "Replace an unavailable or invalid current theme with this desired bundle")
	command.Flags().Bool(offlineFlag, true, "Require rollback to use only retained local state")
	return command
}

func newConfigStatusCommand(newOperations configOperationsFactory) *cobra.Command {
	command := &cobra.Command{
		Use:   "status",
		Short: "Show verified configuration identities and retained artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			operations, err := configOperationsFor(cmd, newOperations)
			if err != nil {
				return err
			}
			asJSON, err := cmd.Flags().GetBool(jsonFlag)
			if err != nil {
				return err
			}
			return operations.Status(cmd.Context(), cmd.OutOrStdout(), configStatusRequest{JSON: asJSON})
		},
	}
	command.Flags().Bool(jsonFlag, false, "Print machine-readable JSON")
	return command
}

func configOperationsFor(cmd *cobra.Command, factory configOperationsFactory) (configOperations, error) {
	if factory == nil {
		return nil, errors.New("config operations are unavailable")
	}
	operations := factory(cmd)
	if operations == nil {
		return nil, errors.New("config operations are unavailable")
	}
	return operations, nil
}

func validateConfigPlan(configPlan plan.InstallationPlan) error {
	if actions := configPlan.ExternalActions(); len(actions) != 0 {
		return fmt.Errorf("config plan contains %d forbidden external action(s)", len(actions))
	}
	return nil
}

func defaultConfigOperations(*cobra.Command) configOperations {
	paths, err := defaultConfigPaths()
	if err != nil {
		return newConfigRuntime(configRuntimeDependencies{initErr: err})
	}
	cache := release.NewArtifactCache(paths.dataRoot)
	return newConfigRuntime(configRuntimeDependencies{
		paths:           paths,
		lock:            &release.Lock{},
		journal:         release.NewJournal(paths.journal),
		resolver:        release.NewArtifactResolver(newReleaseClient(), release.OSFileOps{}, paths.dataRoot),
		admitter:        release.NewArtifactAdmitter(paths.dataRoot),
		cache:           cache,
		dependencyProbe: release.NewOSDependencyProbe(),
		cliVersion:      Version,
	})
}

var configCmd = newConfigCommand(defaultConfigOperations)

func init() {
	rootCmd.AddCommand(configCmd)
}
