package installer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

// HyprlandPlugin identifies one hyprpm-managed plugin and its upstream repo.
type HyprlandPlugin struct {
	Name string
	Repo string
}

// DefaultHyprlandPlugins returns the canonical Hyprland plugin catalog in
// install order. A plugin is enabled after its repo has been added once, so
// plugins sharing a repo never duplicate the add action.
func DefaultHyprlandPlugins() []HyprlandPlugin {
	return []HyprlandPlugin{
		{Name: "hyprbars", Repo: "https://github.com/hyprwm/hyprland-plugins"},
		{Name: "split-monitor-workspaces", Repo: "https://github.com/zjeffer/split-monitor-workspaces"},
	}
}

// HyprlandPluginActions returns the standalone hyprpm sequence in execution
// order for the selected plugins. An empty selection installs the whole
// catalog. The main install plans deliberately do not include these actions
// because hyprpm requires a running Hyprland session; the plugins command
// selects them explicitly.
func HyprlandPluginActions(selected []string) ([]plan.ExternalAction, error) {
	catalog := DefaultHyprlandPlugins()
	if len(selected) == 0 {
		selected = make([]string, len(catalog))
		for i := range catalog {
			selected[i] = catalog[i].Name
		}
	}
	known := make(map[string]HyprlandPlugin, len(catalog))
	for _, plugin := range catalog {
		known[plugin.Name] = plugin
	}
	for _, name := range selected {
		if _, ok := known[name]; !ok {
			valid := make([]string, len(catalog))
			for i := range catalog {
				valid[i] = catalog[i].Name
			}
			return nil, fmt.Errorf("unknown Hyprland plugin %q; valid plugins: %s", name, strings.Join(valid, ", "))
		}
	}

	actions := []plan.ExternalAction{
		action("update Hyprland plugins", "hyprpm", []string{"update"}, "supply-chain", true),
	}
	added := make(map[string]bool, len(catalog))
	for _, plugin := range catalog {
		if slices.Contains(selected, plugin.Name) && !added[plugin.Repo] {
			actions = append(actions, action("add "+plugin.Name, "hyprpm", []string{"add", plugin.Repo}, "supply-chain", true))
			added[plugin.Repo] = true
		}
	}
	for _, plugin := range catalog {
		if slices.Contains(selected, plugin.Name) {
			actions = append(actions, action("enable "+plugin.Name, "hyprpm", []string{"enable", plugin.Name}, "supply-chain", true))
		}
	}
	actions = append(actions, action("reload Hyprland plugins", "hyprpm", []string{"reload"}, "supply-chain", true))
	for i := range actions {
		actions[i].Order = i
	}
	return actions, nil
}

// InstallHyprlandPlugins runs the standalone Hyprland plugin action sequence.
// Deprecated: callers should use the reviewed plugins command instead.
func InstallHyprlandPlugins() error {
	actions, err := HyprlandPluginActions(nil)
	if err != nil {
		return err
	}
	for _, action := range actions {
		if err := runCommand(action.Command.Name, action.Command.Args...); err != nil {
			return fmt.Errorf("%s: %w", action.Description, err)
		}
	}
	return nil
}
