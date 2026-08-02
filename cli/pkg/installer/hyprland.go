package installer

import (
	"fmt"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

// HyprlandPluginActions returns the standalone hyprpm sequence in execution order.
// The install plans deliberately do not include these actions because hyprpm
// requires a running Hyprland session.
func HyprlandPluginActions() []plan.ExternalAction {
	actions := []plan.ExternalAction{
		action("update Hyprland plugins", "hyprpm", []string{"update"}, "supply-chain", true),
		action("add Hyprland plugins", "hyprpm", []string{"add", "https://github.com/hyprwm/hyprland-plugins"}, "supply-chain", true),
		action("add split monitor workspaces", "hyprpm", []string{"add", "https://github.com/zjeffer/split-monitor-workspaces"}, "supply-chain", true),
		action("enable hyprbars", "hyprpm", []string{"enable", "hyprbars"}, "supply-chain", true),
		action("enable split monitor workspaces", "hyprpm", []string{"enable", "split-monitor-workspaces"}, "supply-chain", true),
		action("reload Hyprland plugins", "hyprpm", []string{"reload"}, "supply-chain", true),
	}
	for i := range actions {
		actions[i].Order = i
	}
	return actions
}

// InstallHyprlandPlugins runs the standalone Hyprland plugin action sequence.
// Deprecated: callers should use the reviewed plugins command instead.
func InstallHyprlandPlugins() error {
	for _, action := range HyprlandPluginActions() {
		if err := runCommand(action.Command.Name, action.Command.Args...); err != nil {
			return fmt.Errorf("%s: %w", action.Description, err)
		}
	}
	return nil
}
