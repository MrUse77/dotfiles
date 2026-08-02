package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/MrUse77/dots-cli/pkg/installer"
	"github.com/MrUse77/dots-cli/pkg/installer/external"
	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/ui"
	"github.com/spf13/cobra"
)

const hyprlandInstanceSignatureEnv = "HYPRLAND_INSTANCE_SIGNATURE"

// pluginsDependencies is the injectable boundary for the standalone plugins command.
type pluginsDependencies struct {
	out           io.Writer
	errOut        io.Writer
	input         io.Reader
	lookupEnv     func(string) (string, bool)
	buildPlan     func() (plan.InstallationPlan, error)
	executor      ui.Executor
	programRunner ui.ProgramRunner
}

func defaultPluginsDependencies(cmd *cobra.Command) pluginsDependencies {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	input := cmd.InOrStdin()
	runner := external.NewRunner(nil).WithStdio(input, out, errOut)
	return pluginsDependencies{
		out:           out,
		errOut:        errOut,
		input:         input,
		lookupEnv:     os.LookupEnv,
		buildPlan:     newHyprlandPluginsPlan,
		executor:      installer.NewExternalOnlyExecutor(runner),
		programRunner: nil,
	}
}

func newHyprlandPluginsPlan() (plan.InstallationPlan, error) {
	return plan.NewInstallationPlanWithActions("hyprland-plugins", nil, installer.HyprlandPluginActions())
}

func requireHyprlandInstanceSignature(lookupEnv func(string) (string, bool)) error {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if value, ok := lookupEnv(hyprlandInstanceSignatureEnv); !ok || value == "" {
		return errors.New("HYPRLAND_INSTANCE_SIGNATURE no está disponible; iniciá Hyprland y reintentá con moonarch-cli plugins")
	}
	return nil
}

func runPluginsWithDeps(cmd *cobra.Command, deps pluginsDependencies) error {
	if err := requireHyprlandInstanceSignature(deps.lookupEnv); err != nil {
		return err
	}
	if deps.buildPlan == nil {
		return errors.New("plugins command has no plan factory")
	}
	pluginPlan, err := deps.buildPlan()
	if err != nil {
		return err
	}
	if deps.executor == nil {
		return errors.New("plugins command has no executor")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	rpt, aborted, err := ui.RunWithContext(ctx, pluginPlan, deps.executor, deps.input, deps.out, deps.programRunner)
	if aborted {
		if deps.out != nil {
			fmt.Fprintln(deps.out, "Instalación de plugins cancelada.")
		}
		return nil
	}
	if rpt != nil && deps.out != nil {
		printExecutionReport(deps.out, rpt)
	}
	return err
}

var pluginsCmd = &cobra.Command{
	Use:   "plugins",
	Short: "Instala los plugins de Hyprland con hyprpm",
	Long: `Instala y habilita los plugins de Hyprland en una sesión activa.

Primero iniciá Hyprland y luego revisá el plan antes de confirmar la ejecución.`,
	RunE: runPlugins,
}

func runPlugins(cmd *cobra.Command, _ []string) error {
	return runPluginsWithDeps(cmd, defaultPluginsDependencies(cmd))
}

func init() {
	rootCmd.AddCommand(pluginsCmd)
}
