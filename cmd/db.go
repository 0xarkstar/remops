package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"

	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database operations on services",
}

var flagDBService string

func init() {
	queryCmd := &cobra.Command{
		Use:         "query <service> <sql>",
		Short:       "Run a SQL query on a service's database",
		Annotations: map[string]string{"permission": "viewer"},
		Args:        cobra.ExactArgs(2),
		RunE:        runDBQuery,
	}

	presetCmd := &cobra.Command{
		Use:         "preset <name>",
		Short:       "Run a named SQL preset on a service's database",
		Annotations: map[string]string{"permission": "viewer"},
		Args:        cobra.ExactArgs(1),
		RunE:        runDBPreset,
	}
	presetCmd.Flags().StringVar(&flagDBService, "service", "", "Service name to run the preset against")

	dbCmd.AddCommand(queryCmd, presetCmd)
	rootCmd.AddCommand(dbCmd)
}

// escapeSingleQuotes escapes single quotes for safe use inside a single-quoted shell argument.
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// buildDBExecCmd builds the docker exec command for a DB query.
func buildDBExecCmd(svc config.Service, sql string) (string, error) {
	db := svc.DB
	escaped := escapeSingleQuotes(sql)
	switch db.Engine {
	case "postgresql", "postgres":
		return fmt.Sprintf("docker exec %s psql -U %s -d %s -c '%s'",
			svc.Container, db.User, db.Database, escaped), nil
	case "mysql":
		return fmt.Sprintf("docker exec -e MYSQL_PWD='%s' %s mysql -u %s %s -e '%s'",
			escapeSingleQuotes(db.Password), svc.Container, db.User, db.Database, escaped), nil
	default:
		return "", fmt.Errorf("unsupported db engine %q (supported: postgresql, mysql)", db.Engine)
	}
}

// isWriteQuery delegates to security.IsWriteQuery for centralized SQL classification.
func isWriteQuery(sql string) bool {
	return security.IsWriteQuery(sql)
}

func runDBQuery(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	sql := args[1]

	svc, err := resolveService(serviceName)
	if err != nil {
		return err
	}
	if svc.DB == nil {
		return fmt.Errorf("service %q has no db config", serviceName)
	}

	requiredLevel := config.LevelViewer
	if isWriteQuery(sql) {
		requiredLevel = config.LevelOperator
	}
	if err := security.CheckPermission(currentProfileLevel(), requiredLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitPermissionDenied)
	}

	dockerCmd, err := buildDBExecCmd(svc, sql)
	if err != nil {
		return err
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()

	result, err := tr.Exec(cmd.Context(), svc.Host, dockerCmd)
	if err != nil {
		return fmt.Errorf("db query on %s: %w", serviceName, err)
	}
	fmt.Print(result.Stdout)
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	return nil
}

func runDBPreset(cmd *cobra.Command, args []string) error {
	presetName := args[0]

	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	sql, ok := cfg.Presets[presetName]
	if !ok {
		available := make([]string, 0, len(cfg.Presets))
		for k := range cfg.Presets {
			available = append(available, k)
		}
		if len(available) > 0 {
			return fmt.Errorf("preset %q not found. Available presets: %s", presetName, strings.Join(available, ", "))
		}
		return fmt.Errorf("preset %q not found. No presets defined in config", presetName)
	}

	var serviceName string
	if flagDBService != "" {
		serviceName = flagDBService
	} else {
		// Find first service with a DB config.
		for name, svc := range cfg.Services {
			if svc.DB != nil {
				serviceName = name
				break
			}
		}
		if serviceName == "" {
			return fmt.Errorf("no service with db config found; use --service to specify one")
		}
	}

	// Delegate to runDBQuery with the resolved SQL.
	return runDBQuery(cmd, []string{serviceName, sql})
}
