package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

const runIDLayout = "20060102T150405"

// backupRun is one retained installation run under ~/.dots-backups.
type backupRun struct {
	ID      string
	Time    time.Time
	Targets int
}

// parseRunID extracts the UTC timestamp from a run ID like
// "20260712T120000-abcd1234". Run IDs without a suffix are accepted too.
func parseRunID(id string) (time.Time, bool) {
	ts := id
	if i := strings.Index(id, "-"); i != -1 {
		ts = id[:i]
	}
	t, err := time.Parse(runIDLayout, ts)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// listBackupRuns enumerates run directories under the backup root, newest
// first. Directories that do not parse as run IDs are ignored.
func listBackupRuns(root string) ([]backupRun, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup root: %w", err)
	}
	var runs []backupRun
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, ok := parseRunID(e.Name())
		if !ok {
			continue
		}
		runs = append(runs, backupRun{ID: e.Name(), Time: t})
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].Time.After(runs[j].Time) })
	for i := range runs {
		inv, err := loadInventory(root, runs[i].ID)
		if err != nil {
			runs[i].Targets = 0
			continue
		}
		runs[i].Targets = len(inv.Entries)
	}
	return runs, nil
}

// loadInventory reads and parses the retained inventory of one run.
func loadInventory(root, runID string) (*transaction.Inventory, error) {
	data, err := os.ReadFile(filepath.Join(root, runID, "inventory.json"))
	if err != nil {
		return nil, fmt.Errorf("read inventory for run %s: %w", runID, err)
	}
	var inv transaction.Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("parse inventory for run %s: %w", runID, err)
	}
	return &inv, nil
}

// restoreCandidate is one restorable target from a retained inventory.
//
// Modified means the current destination content differs from what the
// installer wrote (or the installed digest is unknown): restoring will
// overwrite user changes. Removal means the target did not exist before the
// install, so restoring removes the installed destination.
type restoreCandidate struct {
	Entry    transaction.InventoryEntry
	Modified bool
	Removal  bool
}

// restoreCandidates filters the restorable targets of a run: entries that
// were actually mutated. Already restored and ownership-ambiguous entries are
// excluded because their retained artifacts require manual inspection.
func restoreCandidates(inv *transaction.Inventory) ([]restoreCandidate, error) {
	reader := plan.DefaultStateReader()
	var cands []restoreCandidate
	for _, entry := range inv.Entries {
		if entry.State != transaction.EntryMutated {
			continue
		}
		c := restoreCandidate{Entry: entry, Removal: entry.Target.PreState.Type == plan.StateAbsent}
		if !c.Removal {
			state, err := reader.Read(entry.Target.Destination)
			if err != nil {
				return nil, fmt.Errorf("read current state of %q: %w", entry.Target.Destination, err)
			}
			c.Modified = entry.InstalledDigest == "" || state.Digest != entry.InstalledDigest
		}
		cands = append(cands, c)
	}
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].Entry.Target.Destination < cands[j].Entry.Target.Destination
	})
	return cands, nil
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restaura archivos desde los backups de una instalación",
	RunE:  runRestore,
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().String("run", "", "ID del run a restaurar (por defecto se elige interactivamente)")
}

func runRestore(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	root := filepath.Join(homeDir, ".dots-backups")

	runs, err := listBackupRuns(root)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return fmt.Errorf("no hay backups en %s", root)
	}

	runID, err := cmd.Flags().GetString("run")
	if err != nil {
		return err
	}
	if runID == "" {
		opts := make([]huh.Option[string], 0, len(runs))
		for _, r := range runs {
			opts = append(opts, huh.NewOption(
				fmt.Sprintf("%s (%d targets)", r.ID, r.Targets), r.ID))
		}
		if err := huh.NewSelect[string]().
			Title("Seleccioná un run").
			Options(opts...).
			Value(&runID).
			Run(); err != nil {
			return err
		}
	}

	inv, err := loadInventory(root, runID)
	if err != nil {
		return err
	}
	cands, err := restoreCandidates(inv)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return fmt.Errorf("el run %s no tiene targets restaurables", runID)
	}

	labels := make([]huh.Option[int], 0, len(cands))
	for i, c := range cands {
		label := c.Entry.Target.Destination
		switch {
		case c.Removal:
			label += " (sin backup — se eliminará)"
		case c.Modified:
			label += " (modificado)"
		}
		labels = append(labels, huh.NewOption(label, i))
	}
	var selected []int
	if err := huh.NewMultiSelect[int]().
		Title("Targets a restaurar").
		Options(labels...).
		Value(&selected).
		Run(); err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Fprintln(out, "No se seleccionó ningún target.")
		return nil
	}

	modified := 0
	for _, i := range selected {
		if cands[i].Modified {
			modified++
		}
	}
	if modified > 0 {
		var confirm bool
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("%d target(s) fueron modificados después de la instalación y serán pisados. ¿Continuar?", modified)).
			Value(&confirm).
			Run(); err != nil {
			return err
		}
		if !confirm {
			fmt.Fprintln(out, "Restauración cancelada.")
			return nil
		}
	}

	var confirmAll bool
	if err := huh.NewConfirm().
		Title(fmt.Sprintf("¿Restaurar %d target(s)?", len(selected))).
		Value(&confirmAll).
		Run(); err != nil {
		return err
	}
	if !confirmAll {
		fmt.Fprintln(out, "Restauración cancelada.")
		return nil
	}

	targets := make([]plan.Target, 0, len(selected))
	for _, i := range selected {
		targets = append(targets, cands[i].Entry.Target)
	}
	p, err := plan.NewInstallationPlan(inv.RunID, targets)
	if err != nil {
		return err
	}
	tx := transaction.New(p)
	for _, tgt := range targets {
		if err := tx.RestoreTarget(tgt); err != nil {
			return fmt.Errorf("restaurar %s: %w", tgt.Destination, err)
		}
		fmt.Fprintf(out, "restaurado %s\n", tgt.Destination)
	}
	return nil
}
