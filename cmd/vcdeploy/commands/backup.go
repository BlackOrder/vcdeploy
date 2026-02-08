package commands

import (
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

	backupExportCmd.Flags().StringP("output", "o", "", "Output file path for the export")
	backupExportCmd.Flags().String("passphrase", "", "Passphrase for encryption (prompted if not provided)")
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

	if string(pass1) != string(pass2) {
		return "", fmt.Errorf("passphrases do not match")
	}

	return string(pass1), nil
}
