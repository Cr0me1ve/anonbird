package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/netbirdio/netbird/client/internal/anonymous"
)

const migrationManifestName = "manifest.json"

var (
	migrateApply        bool
	migrateDryRun       bool
	migrateRoot         string
	migrateBackupDir    string
	migrateForce        bool
	migrateNoBackup     bool
	migrateCompatLink   bool
	migrateRejoin       string
	migrateServerScript string
	migrateServerDir    string
	migrateAssumeYes    bool
)

type migrationOptions struct {
	Apply        bool
	DryRun       bool
	Root         string
	BackupDir    string
	Force        bool
	NoBackup     bool
	CompatLink   bool
	Rejoin       string
	ServerScript string
	ServerDir    string
	AssumeYes    bool

	AllowUnsafeClearnet bool
	UnsafeClearnetAck   bool
}

type migrationAction struct {
	Kind        string `json:"kind"`
	Source      string `json:"source,omitempty"`
	Target      string `json:"target,omitempty"`
	Description string `json:"description"`
	Rewrite     bool   `json:"rewrite,omitempty"`
	LinkTarget  string `json:"link_target,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
}

type migrationBackupEntry struct {
	Target  string `json:"target"`
	Backup  string `json:"backup,omitempty"`
	Existed bool   `json:"existed"`
}

type migrationSourceBackup struct {
	Source string `json:"source"`
	Backup string `json:"backup"`
}

type migrationManifest struct {
	Version       int                     `json:"version"`
	CreatedAt     string                  `json:"created_at"`
	Scope         string                  `json:"scope"`
	Root          string                  `json:"root"`
	Actions       []migrationAction       `json:"actions"`
	TargetBackups []migrationBackupEntry  `json:"target_backups"`
	SourceBackups []migrationSourceBackup `json:"source_backups"`
}

type migrationPlan struct {
	Scope     string
	BackupDir string
	Actions   []migrationAction
	Notes     []string
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate NetBird installations to AnonBird",
	Long: `Migrate NetBird installations to AnonBird.

The default mode is a dry run. Use --apply to perform file and service changes.
Client migration is implemented natively for Linux-style paths. Server migration
delegates to the packaged AnonBird self-host migration script, which migrates a
legacy NetBird Docker Compose deployment to the combined AnonBird server stack.`,
}

var migrateClientCmd = &cobra.Command{
	Use:   "client",
	Short: "Migrate a Linux NetBird client installation",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := currentMigrationOptions()
		if err != nil {
			return err
		}
		if opts.Rejoin != "" {
			if _, err := parseJoinToken(opts.Rejoin); err != nil {
				return fmt.Errorf("validate --rejoin token: %w", err)
			}
		}

		plan, err := buildClientMigrationPlan(opts)
		if err != nil {
			return err
		}
		printMigrationPlan(cmd.OutOrStdout(), plan, opts)
		if opts.DryRun {
			return nil
		}
		return applyClientMigrationPlan(cmd.OutOrStdout(), plan, opts)
	},
}

var migrateServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Migrate a self-hosted NetBird server deployment",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := currentMigrationOptions()
		if err != nil {
			return err
		}
		return runServerMigrationScript(cmd.OutOrStdout(), opts)
	},
}

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback a previous AnonBird client migration backup",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := currentMigrationOptions()
		if err != nil {
			return err
		}
		if opts.BackupDir == "" {
			backupDir, err := findLatestClientMigrationBackup(opts)
			if err != nil {
				return err
			}
			opts.BackupDir = backupDir
		}

		if opts.DryRun {
			manifest, err := readMigrationManifest(opts.mapPath(opts.BackupDir))
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "AnonBird rollback dry-run for %s\n", opts.BackupDir)
			for _, entry := range manifest.TargetBackups {
				if entry.Existed {
					fmt.Fprintf(cmd.OutOrStdout(), "  restore %s from backup\n", entry.Target)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  remove migrated target %s\n", entry.Target)
				}
			}
			return nil
		}

		return rollbackClientMigration(cmd.OutOrStdout(), opts)
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateClientCmd, migrateServerCmd, migrateRollbackCmd)

	migrateCmd.PersistentFlags().BoolVar(&migrateApply, "apply", false, "apply migration changes")
	migrateCmd.PersistentFlags().BoolVar(&migrateDryRun, "dry-run", false, "show planned changes without applying them (default unless --apply is set)")
	migrateCmd.PersistentFlags().StringVar(&migrateRoot, "root", "/", "filesystem root to migrate; use a temp root for tests")
	migrateCmd.PersistentFlags().StringVar(&migrateBackupDir, "backup-dir", "", "backup directory; defaults to /var/backups/anonbird/migration-<timestamp>")
	migrateCmd.PersistentFlags().BoolVar(&migrateForce, "force", false, "allow overwriting existing AnonBird targets after backing them up")
	migrateCmd.PersistentFlags().BoolVar(&migrateNoBackup, "no-backup", false, "skip backup; requires --force")
	migrateClientCmd.Flags().BoolVar(&migrateCompatLink, "compat-symlink", false, "create a temporary netbird -> anonbird compatibility symlink when possible")
	migrateClientCmd.Flags().StringVar(&migrateRejoin, "rejoin", "", "run anonbird join with this anonbird://join?... token after applying migration")
	migrateServerCmd.Flags().StringVar(&migrateServerScript, "script", "", "path to the AnonBird server migration script")
	migrateServerCmd.Flags().StringVar(&migrateServerDir, "install-dir", "", "path to the existing NetBird server installation")
	migrateServerCmd.Flags().BoolVarP(&migrateAssumeYes, "yes", "y", false, "run server migration non-interactively")
}

func currentMigrationOptions() (migrationOptions, error) {
	if migrateApply && migrateDryRun {
		return migrationOptions{}, fmt.Errorf("--apply and --dry-run are mutually exclusive")
	}
	if migrateNoBackup && !migrateForce {
		return migrationOptions{}, fmt.Errorf("--no-backup requires --force")
	}

	opts := migrationOptions{
		Apply:        migrateApply,
		DryRun:       !migrateApply,
		Root:         firstNonEmpty(migrateRoot, "/"),
		BackupDir:    migrateBackupDir,
		Force:        migrateForce,
		NoBackup:     migrateNoBackup,
		CompatLink:   migrateCompatLink,
		Rejoin:       strings.TrimSpace(migrateRejoin),
		ServerScript: strings.TrimSpace(migrateServerScript),
		ServerDir:    strings.TrimSpace(migrateServerDir),
		AssumeYes:    migrateAssumeYes,

		AllowUnsafeClearnet: allowUnsafeClearnet,
		UnsafeClearnetAck:   unsafeClearnetAck,
	}
	return opts, nil
}

func buildClientMigrationPlan(opts migrationOptions) (migrationPlan, error) {
	backupDir := opts.BackupDir
	if backupDir == "" && !opts.NoBackup {
		backupDir = "/var/backups/anonbird/migration-" + time.Now().UTC().Format("20060102-150405")
	}
	plan := migrationPlan{
		Scope:     "client",
		BackupDir: opts.mapPath(backupDir),
	}

	for _, pair := range []struct {
		source string
		target string
		desc   string
	}{
		{"/etc/netbird", "/etc/anonbird", "copy client configuration"},
		{"/var/lib/netbird", "/var/lib/anonbird", "copy client state"},
		{"/var/log/netbird", "/var/log/anonbird", "copy client logs"},
	} {
		if opts.pathExists(pair.source) {
			plan.Actions = append(plan.Actions, migrationAction{
				Kind:        "copy",
				Source:      pair.source,
				Target:      pair.target,
				Description: pair.desc,
			})
		}
	}

	for _, source := range []string{
		"/etc/systemd/system/netbird.service",
		"/lib/systemd/system/netbird.service",
		"/usr/lib/systemd/system/netbird.service",
	} {
		if opts.pathExists(source) {
			plan.Actions = append(plan.Actions, migrationAction{
				Kind:        "copy",
				Source:      source,
				Target:      "/etc/systemd/system/anonbird.service",
				Description: "copy and rewrite systemd service unit",
				Rewrite:     true,
			})
			break
		}
	}

	if opts.CompatLink {
		plan.Actions = append(plan.Actions, migrationAction{
			Kind:        "symlink",
			Target:      "/usr/local/bin/netbird",
			LinkTarget:  "/usr/local/bin/anonbird",
			Description: "create temporary netbird compatibility symlink",
			Optional:    true,
		})
	}

	if opts.Rejoin != "" {
		plan.Notes = append(plan.Notes, "validated --rejoin token; apply mode will run anonbird join after file migration when migrating the live root")
	}
	if unsafeConfigs, err := findUnsafeMigratedClientConfigs(opts); err != nil {
		return migrationPlan{}, err
	} else if len(unsafeConfigs) > 0 {
		plan.Notes = append(plan.Notes, fmt.Sprintf("found %d legacy non-anonymous NetBird config file(s); apply requires --rejoin or explicit unsafe clearnet confirmation", len(unsafeConfigs)))
	}
	if len(plan.Actions) == 0 {
		plan.Notes = append(plan.Notes, "no legacy NetBird client paths were found")
	}
	return plan, nil
}

func printMigrationPlan(out io.Writer, plan migrationPlan, opts migrationOptions) {
	mode := "dry-run"
	if opts.Apply {
		mode = "apply"
	}
	fmt.Fprintf(out, "AnonBird %s migration plan (%s)\n", plan.Scope, mode)
	if opts.NoBackup {
		fmt.Fprintln(out, "Backup: disabled by --no-backup")
	} else {
		fmt.Fprintf(out, "Backup: %s\n", plan.BackupDir)
	}
	for _, action := range plan.Actions {
		switch action.Kind {
		case "copy":
			fmt.Fprintf(out, "  - %s: %s -> %s\n", action.Description, action.Source, action.Target)
		case "symlink":
			fmt.Fprintf(out, "  - %s: %s -> %s\n", action.Description, action.Target, action.LinkTarget)
		default:
			fmt.Fprintf(out, "  - %s\n", action.Description)
		}
	}
	for _, note := range plan.Notes {
		fmt.Fprintf(out, "  note: %s\n", note)
	}
	if opts.DryRun {
		fmt.Fprintln(out, "Dry-run only. Re-run with --apply to make changes.")
	}
}

func applyClientMigrationPlan(out io.Writer, plan migrationPlan, opts migrationOptions) error {
	if len(plan.Actions) == 0 {
		fmt.Fprintln(out, "Nothing to migrate.")
		return nil
	}
	if err := validateClientMigrationSafety(opts); err != nil {
		return err
	}
	if !opts.NoBackup {
		if err := os.MkdirAll(plan.BackupDir, 0o700); err != nil {
			return fmt.Errorf("create backup dir: %w", err)
		}
	}

	if opts.Root == "/" {
		runSystemctl(out, false, "stop", "netbird.service")
		runSystemctl(out, false, "disable", "netbird.service")
	}

	manifest := migrationManifest{
		Version:   1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Scope:     plan.Scope,
		Root:      opts.Root,
		Actions:   plan.Actions,
	}

	for _, action := range plan.Actions {
		switch action.Kind {
		case "copy":
			if err := backupSource(action.Source, &manifest, opts, plan.BackupDir); err != nil {
				return err
			}
			if err := backupTarget(action.Target, &manifest, opts, plan.BackupDir); err != nil {
				return err
			}
			if err := copyAction(action, opts); err != nil {
				return err
			}
			fmt.Fprintf(out, "Migrated %s -> %s\n", action.Source, action.Target)
		case "symlink":
			if err := backupTarget(action.Target, &manifest, opts, plan.BackupDir); err != nil {
				return err
			}
			if err := createSymlinkAction(action, opts); err != nil {
				if action.Optional {
					fmt.Fprintf(out, "Warning: skipped optional symlink %s: %v\n", action.Target, err)
					continue
				}
				return err
			}
			fmt.Fprintf(out, "Created symlink %s -> %s\n", action.Target, action.LinkTarget)
		}
	}

	if opts.Rejoin != "" {
		token, err := parseJoinToken(opts.Rejoin)
		if err != nil {
			return fmt.Errorf("validate --rejoin token: %w", err)
		}
		if err := hardenMigratedClientConfigs(out, plan, opts, token); err != nil {
			return err
		}
	}

	if !opts.NoBackup {
		if err := writeMigrationManifest(plan.BackupDir, manifest); err != nil {
			return err
		}
		fmt.Fprintf(out, "Backup manifest written to %s\n", filepath.Join(plan.BackupDir, migrationManifestName))
	}

	if opts.Root == "/" {
		runSystemctl(out, false, "daemon-reload")
		runSystemctl(out, false, "enable", "anonbird.service")
		runSystemctl(out, false, "start", "anonbird.service")
		if opts.Rejoin != "" {
			if err := runAnonBirdJoin(out, opts.Rejoin); err != nil {
				return err
			}
		}
	} else if opts.Rejoin != "" {
		fmt.Fprintln(out, "Skipping --rejoin because --root is not /. Run anonbird join manually on the target host.")
	}

	fmt.Fprintln(out, "Client migration applied.")
	return nil
}

func validateClientMigrationSafety(opts migrationOptions) error {
	unsafeConfigs, err := findUnsafeMigratedClientConfigs(opts)
	if err != nil {
		return err
	}
	if len(unsafeConfigs) == 0 {
		return nil
	}
	if opts.Rejoin != "" {
		return nil
	}
	if opts.AllowUnsafeClearnet && opts.UnsafeClearnetAck {
		return nil
	}

	return fmt.Errorf("client migration would copy non-anonymous NetBird config(s): %s. Provide --rejoin \"anonbird://join?...\" to migrate into anonymous mode, or pass --%s and --%s to allow unsafe clearnet migration", strings.Join(unsafeConfigs, ", "), allowUnsafeClearnetFlag, unsafeClearnetAckFlag)
}

func findUnsafeMigratedClientConfigs(opts migrationOptions) ([]string, error) {
	var unsafeConfigs []string
	for _, root := range []string{"/etc/netbird", "/var/lib/netbird"} {
		mappedRoot := opts.mapPath(root)
		if _, err := os.Lstat(mappedRoot); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("inspect %s: %w", root, err)
		}
		err := filepath.WalkDir(mappedRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				return nil
			}
			unsafe, err := isUnsafeClientConfig(path)
			if err != nil {
				return fmt.Errorf("inspect client config %s: %w", path, err)
			}
			if unsafe {
				unsafeConfigs = append(unsafeConfigs, opts.logicalPath(path))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(unsafeConfigs)
	return unsafeConfigs, nil
}

func isUnsafeClientConfig(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, nil
	}
	if !hasClientManagementURL(raw) {
		return false, nil
	}
	if anonymousMode, ok := raw["anonymous_mode"].(bool); ok && anonymousMode {
		return false, nil
	}
	return true, nil
}

func hasClientManagementURL(raw map[string]any) bool {
	for _, key := range []string{"ManagementURL", "management_url", "managementURL"} {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed) != ""
		case map[string]any:
			return len(typed) > 0
		default:
			return value != nil
		}
	}
	return false
}

func hardenMigratedClientConfigs(out io.Writer, plan migrationPlan, opts migrationOptions, token joinToken) error {
	var updated []string
	for _, action := range plan.Actions {
		if action.Kind != "copy" || action.Target != "/etc/anonbird" {
			continue
		}
		root := opts.mapPath(action.Target)
		if _, err := os.Lstat(root); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("inspect migrated config dir: %w", err)
		}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				return nil
			}
			changed, err := hardenMigratedClientConfig(path, token)
			if err != nil {
				return fmt.Errorf("harden migrated config %s: %w", path, err)
			}
			if changed {
				updated = append(updated, opts.logicalPath(path))
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	for _, path := range updated {
		fmt.Fprintf(out, "Hardened migrated config for anonymous rejoin: %s\n", path)
	}
	return nil
}

func hardenMigratedClientConfig(path string, token joinToken) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, nil
	}
	if !hasClientManagementURL(raw) {
		return false, nil
	}

	transport := anonymous.NormalizeTransport(token.Transport)
	raw["ManagementURL"] = token.ManagementURL
	raw["anonymous_mode"] = true
	raw["anonymous_transport"] = transport
	raw["DisableAutoConnect"] = true

	rewritten, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return false, err
	}
	rewritten = append(rewritten, '\n')
	return true, os.WriteFile(path, rewritten, 0o600)
}

func backupSource(source string, manifest *migrationManifest, opts migrationOptions, backupDir string) error {
	if opts.NoBackup || !opts.pathExists(source) {
		return nil
	}
	backup := filepath.Join(backupDir, "source", strings.TrimPrefix(filepath.Clean(source), string(os.PathSeparator)))
	if err := copyPath(opts.mapPath(source), backup, true); err != nil {
		return fmt.Errorf("backup source %s: %w", source, err)
	}
	manifest.SourceBackups = append(manifest.SourceBackups, migrationSourceBackup{Source: source, Backup: backup})
	return nil
}

func backupTarget(target string, manifest *migrationManifest, opts migrationOptions, backupDir string) error {
	exists := opts.pathExists(target)
	entry := migrationBackupEntry{Target: target, Existed: exists}
	if exists {
		if opts.NoBackup {
			manifest.TargetBackups = append(manifest.TargetBackups, entry)
			return nil
		}
		backup := filepath.Join(backupDir, "target", strings.TrimPrefix(filepath.Clean(target), string(os.PathSeparator)))
		if err := copyPath(opts.mapPath(target), backup, true); err != nil {
			return fmt.Errorf("backup target %s: %w", target, err)
		}
		entry.Backup = backup
	}
	manifest.TargetBackups = append(manifest.TargetBackups, entry)
	return nil
}

func copyAction(action migrationAction, opts migrationOptions) error {
	source := opts.mapPath(action.Source)
	target := opts.mapPath(action.Target)
	if _, err := os.Lstat(target); err == nil {
		if !opts.Force {
			return fmt.Errorf("target exists: %s (use --force after reviewing the backup plan)", action.Target)
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove existing target %s: %w", action.Target, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect target %s: %w", action.Target, err)
	}

	if action.Rewrite {
		return copyRewrittenFile(source, target)
	}
	return copyPath(source, target, true)
}

func createSymlinkAction(action migrationAction, opts migrationOptions) error {
	target := opts.mapPath(action.Target)
	linkTarget := action.LinkTarget
	if opts.Root != "/" {
		linkTarget = opts.mapPath(action.LinkTarget)
	}
	if _, err := os.Lstat(linkTarget); err != nil {
		return fmt.Errorf("link target does not exist: %s", action.LinkTarget)
	}
	if _, err := os.Lstat(target); err == nil {
		if !opts.Force {
			return fmt.Errorf("target exists: %s", action.Target)
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.Symlink(linkTarget, target)
}

func rollbackClientMigration(out io.Writer, opts migrationOptions) error {
	manifest, err := readMigrationManifest(opts.mapPath(opts.BackupDir))
	if err != nil {
		return err
	}
	if manifest.Scope != "client" {
		return fmt.Errorf("backup scope is %q, expected client", manifest.Scope)
	}
	if opts.Root == "/" {
		runSystemctl(out, false, "stop", "anonbird.service")
	}

	for i := len(manifest.TargetBackups) - 1; i >= 0; i-- {
		entry := manifest.TargetBackups[i]
		target := opts.mapPath(entry.Target)
		if entry.Existed {
			if entry.Backup == "" {
				return fmt.Errorf("backup entry for %s has no backup path", entry.Target)
			}
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("remove migrated target %s: %w", entry.Target, err)
			}
			if err := copyPath(entry.Backup, target, true); err != nil {
				return fmt.Errorf("restore %s: %w", entry.Target, err)
			}
			fmt.Fprintf(out, "Restored %s\n", entry.Target)
			continue
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove migrated target %s: %w", entry.Target, err)
		}
		fmt.Fprintf(out, "Removed migrated target %s\n", entry.Target)
	}

	if opts.Root == "/" {
		runSystemctl(out, false, "daemon-reload")
		runSystemctl(out, false, "start", "netbird.service")
	}
	fmt.Fprintln(out, "Client migration rollback applied.")
	return nil
}

func readMigrationManifest(backupDir string) (migrationManifest, error) {
	if backupDir == "" {
		return migrationManifest{}, fmt.Errorf("--backup-dir is required for rollback unless a latest backup can be detected")
	}
	data, err := os.ReadFile(filepath.Join(backupDir, migrationManifestName))
	if err != nil {
		return migrationManifest{}, fmt.Errorf("read migration manifest: %w", err)
	}
	var manifest migrationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return migrationManifest{}, fmt.Errorf("parse migration manifest: %w", err)
	}
	return manifest, nil
}

func writeMigrationManifest(backupDir string, manifest migrationManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(backupDir, migrationManifestName), data, 0o600)
}

func findLatestClientMigrationBackup(opts migrationOptions) (string, error) {
	logicalBase := "/var/backups/anonbird"
	base := opts.mapPath(logicalBase)
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("find migration backups: %w", err)
	}
	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "migration-") {
			continue
		}
		manifestPath := filepath.Join(base, entry.Name(), migrationManifestName)
		manifest, err := readMigrationManifest(filepath.Dir(manifestPath))
		if err == nil && manifest.Scope == "client" {
			candidates = append(candidates, filepath.Join(logicalBase, entry.Name()))
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no client migration backups found under %s", base)
	}
	sort.Strings(candidates)
	return candidates[len(candidates)-1], nil
}

func runServerMigrationScript(out io.Writer, opts migrationOptions) error {
	script, err := locateServerMigrationScript(opts)
	if err != nil {
		return err
	}
	args := []string{script}
	if opts.ServerDir != "" {
		args = append(args, "--install-dir", opts.ServerDir)
	}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.AssumeYes || opts.Apply {
		args = append(args, "--non-interactive")
	}
	fmt.Fprintf(out, "Running server migration script: bash %s\n", strings.Join(args, " "))
	cmd := exec.Command("bash", args...)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func locateServerMigrationScript(opts migrationOptions) (string, error) {
	var candidates []string
	if opts.ServerScript != "" {
		candidates = append(candidates, opts.ServerScript)
	}
	if env := strings.TrimSpace(os.Getenv("ANONBIRD_SERVER_MIGRATE_SCRIPT")); env != "" {
		candidates = append(candidates, env)
	}
	cwd, _ := os.Getwd()
	if cwd != "" {
		candidates = append(candidates,
			filepath.Join(cwd, "infrastructure_files", "migrate.sh"),
			filepath.Join(cwd, "migrate.sh"),
		)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "migrate.sh"),
			filepath.Join(exeDir, "anonbird-migrate-server.sh"),
		)
	}
	candidates = append(candidates,
		"/usr/share/anonbird/migrate.sh",
		"/opt/anonbird/migrate.sh",
	)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("server migration script not found; pass --script or install /usr/share/anonbird/migrate.sh")
}

func runSystemctl(out io.Writer, required bool, args ...string) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		if required {
			fmt.Fprintf(out, "Warning: systemctl not found: %v\n", err)
		}
		return
	}
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && required {
		fmt.Fprintf(out, "Warning: systemctl %s failed: %v\n%s\n", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}

func runAnonBirdJoin(out io.Writer, token string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable for --rejoin: %w", err)
	}
	cmd := exec.Command(exe, "join", token)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run anonbird join: %w", err)
	}
	return nil
}

func (o migrationOptions) mapPath(p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) || o.Root == "" || o.Root == "/" {
		return filepath.Clean(p)
	}
	return filepath.Join(filepath.Clean(o.Root), strings.TrimPrefix(filepath.Clean(p), string(os.PathSeparator)))
}

func (o migrationOptions) logicalPath(mappedPath string) string {
	cleaned := filepath.Clean(mappedPath)
	root := filepath.Clean(o.Root)
	if root == "" || root == "/" {
		return cleaned
	}
	if rel, err := filepath.Rel(root, cleaned); err == nil && !strings.HasPrefix(rel, "..") {
		return string(os.PathSeparator) + rel
	}
	return cleaned
}

func (o migrationOptions) pathExists(p string) bool {
	_, err := os.Lstat(o.mapPath(p))
	return err == nil
}

func copyRewrittenFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	rewritten := rewriteNetBirdUnit(string(data))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(rewritten), 0o644)
}

func rewriteNetBirdUnit(content string) string {
	replacer := strings.NewReplacer(
		"NetBird", "AnonBird",
		"netbird service run", "anonbird service run",
		"/usr/bin/netbird", "/usr/bin/anonbird",
		"/usr/local/bin/netbird", "/usr/local/bin/anonbird",
		"/etc/netbird", "/etc/anonbird",
		"/var/lib/netbird", "/var/lib/anonbird",
		"/var/log/netbird", "/var/log/anonbird",
		"/var/run/netbird.sock", "/var/run/anonbird.sock",
		"SYSTEMD_UNIT=netbird", "SYSTEMD_UNIT=anonbird",
	)
	return replacer.Replace(content)
}

func copyPath(source, target string, overwrite bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		if !overwrite {
			return fmt.Errorf("target exists: %s", target)
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		link, err := os.Readlink(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.Symlink(link, target)
	case info.IsDir():
		return copyDir(source, target, info.Mode().Perm())
	default:
		return copyFile(source, target, info.Mode().Perm())
	}
}

func copyDir(source, target string, mode fs.FileMode) error {
	if err := os.MkdirAll(target, mode); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(source, entry.Name())
		dst := filepath.Join(target, entry.Name())
		if err := copyPath(src, dst, true); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}
