package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/backup"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(backupExportCmd)
	rootCmd.AddCommand(backupImportCmd)

	backupExportCmd.Flags().StringP("output", "o", "", "Output file path for the export")
	backupExportCmd.Flags().String("passphrase", "", "Passphrase for encryption (prompted if not provided)")

	backupImportCmd.Flags().StringP("input", "i", "", "Input file path for the import (required)")
	backupImportCmd.Flags().String("passphrase", "", "Passphrase for decryption (prompted if not provided)")
	backupImportCmd.Flags().String("strategy", "", "Strategy for all tables: merge-all or replace-all (interactive if not set)")
	_ = backupImportCmd.MarkFlagRequired("input")
}

var backupExportCmd = &cobra.Command{
	Use:   "backup-export",
	Short: "Export database with passphrase-protected encryption",
	Long: `Creates a passphrase-protected export of all importable configuration.

Sensitive fields (secrets, private keys, credentials) are decrypted from the
server's KMS/MasterKey and re-encrypted with the provided passphrase using
Argon2id + AES-256-GCM.

The export file is a standalone SQLite database that can be imported on
another vcdeploy server using 'vcdeploy backup-import'.

KMS key material is NEVER included in the export.`,
	RunE: runBackupExport,
}

func runBackupExport(cmd *cobra.Command, _ []string) error {
	svc, cleanup, err := initServices()
	if err != nil {
		return fmt.Errorf("initialize services: %w", err)
	}
	defer cleanup()

	// Get output path
	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		timestamp := time.Now().Format("20060102-150405")
		outputPath = fmt.Sprintf("vcdeploy-export-%s.db", timestamp)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0o750); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	// Get passphrase
	passphrase, _ := cmd.Flags().GetString("passphrase")
	if passphrase == "" {
		passphrase, err = promptPassphrase()
		if err != nil {
			return fmt.Errorf("read passphrase: %w", err)
		}
	}

	if len(passphrase) < 8 {
		return fmt.Errorf("passphrase must be at least 8 characters")
	}

	fmt.Printf("Exporting to: %s\n", outputPath)

	exportSvc := backup.NewExportService(svc.Store(), svc.KMS(), svc.MasterKey(), svc.logger)
	if err := exportSvc.Export(cmd.Context(), passphrase, outputPath); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	// Get file size
	info, err := os.Stat(outputPath)
	if err == nil {
		fmt.Printf("✅ Export completed successfully (%d KB)\n", info.Size()/1024)
	} else {
		fmt.Println("✅ Export completed successfully")
	}

	return nil
}

// promptPassphrase prompts the user for a passphrase with confirmation.
func promptPassphrase() (string, error) {
	fmt.Print("Enter passphrase: ")
	pass1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}

	fmt.Print("Confirm passphrase: ")
	pass2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}

	if !bytes.Equal(pass1, pass2) {
		return "", fmt.Errorf("passphrases do not match")
	}

	return string(pass1), nil
}

var backupImportCmd = &cobra.Command{
	Use:   "backup-import",
	Short: "Import data from a passphrase-protected export file",
	Long: `Imports configuration from a vcdeploy export file.

The server must be in maintenance mode before importing. This prevents
concurrent writes that could conflict with the import process.

In interactive mode (default), you'll see a diff of changes and choose
a strategy (merge/replace/skip) for each table.

In non-interactive mode, use --strategy merge-all or --strategy replace-all
to apply a uniform strategy to all tables.`,
	RunE: runBackupImport,
}

func runBackupImport(cmd *cobra.Command, _ []string) error {
	svc, cleanup, err := initServices()
	if err != nil {
		return fmt.Errorf("initialize services: %w", err)
	}
	defer cleanup()

	inputPath, _ := cmd.Flags().GetString("input")

	// Check file exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("import file not found: %s", inputPath)
	}

	// Get passphrase
	passphrase, _ := cmd.Flags().GetString("passphrase")
	if passphrase == "" {
		fmt.Print("Enter passphrase: ")
		pass, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("read passphrase: %w", err)
		}
		passphrase = string(pass)
	}

	importSvc := backup.NewImportService(svc.Store(), svc.KMS(), svc.MasterKey(), svc.logger)

	// Compute diff
	fmt.Println("Analyzing import file...")
	diff, err := importSvc.ComputeDiff(cmd.Context(), inputPath)
	if err != nil {
		return fmt.Errorf("compute diff: %w", err)
	}

	// Display diff
	fmt.Println()
	fmt.Println("Import Diff:")
	fmt.Println("─────────────────────────────────────────────────")
	fmt.Printf("%-30s %6s %6s %6s %6s\n", "Table", "New", "Chg", "Main", "Total")
	fmt.Println("─────────────────────────────────────────────────")
	for _, td := range diff.Tables {
		if td.Total > 0 || td.OnlyInMain > 0 {
			fmt.Printf("%-30s %6d %6d %6d %6d\n", td.Name, td.NewRecords, td.Changed, td.OnlyInMain, td.Total)
		}
	}
	fmt.Println("─────────────────────────────────────────────────")

	// Build strategies
	strategy, _ := cmd.Flags().GetString("strategy")
	strategies := make(map[string]backup.ImportStrategy)

	switch strategy {
	case "merge-all":
		for _, td := range diff.Tables {
			strategies[td.Name] = backup.StrategyMerge
		}
		fmt.Println("Strategy: merge all tables")

	case "replace-all":
		for _, td := range diff.Tables {
			strategies[td.Name] = backup.StrategyReplace
		}
		fmt.Println("Strategy: replace all tables")

	default:
		// Interactive mode
		fmt.Println()
		fmt.Println("Choose strategy per table ([m]erge / [r]eplace / [s]kip):")
		for _, td := range diff.Tables {
			if td.Total == 0 && td.OnlyInMain == 0 {
				strategies[td.Name] = backup.StrategySkip
				continue
			}
			for {
				fmt.Printf("  %-30s → ", td.Name)
				var choice string
				if _, err := fmt.Scanln(&choice); err != nil {
					choice = "s"
				}
				switch choice {
				case "m", "merge":
					strategies[td.Name] = backup.StrategyMerge
				case "r", "replace":
					strategies[td.Name] = backup.StrategyReplace
				case "s", "skip", "":
					strategies[td.Name] = backup.StrategySkip
				default:
					fmt.Println("    Invalid choice. Use m(erge), r(eplace), or s(kip)")
					continue
				}
				break
			}
		}
	}

	fmt.Println()
	fmt.Println("Executing import...")

	if err := importSvc.Execute(cmd.Context(), inputPath, passphrase, strategies); err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	fmt.Println("✅ Import completed successfully")
	return nil
}
