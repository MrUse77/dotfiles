package installer

import (
	"os/exec"
	"strings"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

const powerProfilesService = "power-profiles-daemon.service"

// PowerProfilesState is the read-only systemd state used to select safe
// power-profile actions.
type PowerProfilesState struct {
	TLPActive bool
	Masked    bool
}

type systemctlOutputFunc func(args ...string) (string, error)

// DetectPowerProfiles reads the state needed by the install plan. It does not
// change service state or require elevated privileges.
func DetectPowerProfiles() PowerProfilesState {
	return detectPowerProfiles(systemctlOutput)
}

func detectPowerProfiles(output systemctlOutputFunc) PowerProfilesState {
	_, tlpErr := output("is-active", "--quiet", "tlp")
	state, _ := output("is-enabled", powerProfilesService)
	return PowerProfilesState{
		TLPActive: tlpErr == nil,
		Masked:    isMaskedUnitState(state),
	}
}

func isMaskedUnitState(state string) bool {
	return strings.HasPrefix(strings.TrimSpace(state), "masked")
}

func powerProfilesActions(state PowerProfilesState) []plan.ExternalAction {
	if state.TLPActive {
		return nil
	}

	actions := make([]plan.ExternalAction, 0, 2)
	if state.Masked {
		actions = append(actions, action(
			"unmask power profiles",
			"sudo",
			[]string{"systemctl", "unmask", powerProfilesService},
			"privileged",
			true,
		))
	}
	actions = append(actions, action(
		"enable power profiles",
		"sudo",
		[]string{"systemctl", "enable", "--now", powerProfilesService},
		"privileged",
		true,
	))
	return actions
}

func systemctlOutput(args ...string) (string, error) {
	output, err := exec.Command("systemctl", args...).Output()
	return string(output), err
}
