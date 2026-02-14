package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsCmd = &cobra.Command{
	Use:    "docs",
	Short:  "Generate documentation",
	Long:   "Generate CLI documentation in various formats.",
	Hidden: true, // Hidden from normal help output
}

var docsMarkdownCmd = &cobra.Command{
	Use:   "markdown [output-dir]",
	Short: "Generate markdown documentation",
	Long: `Generate markdown documentation for all CLI commands.

The generated files can be included in your documentation website
or used with static site generators like docsify, Hugo, or MkDocs.

Example:
  vcdeploy docs markdown docs/reference/cli/`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir := args[0]

		// Create output directory if it doesn't exist
		// #nosec G301 - Documentation output directory needs world-read access
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		// Generate markdown documentation
		err := doc.GenMarkdownTree(rootCmd, outputDir)
		if err != nil {
			return fmt.Errorf("generating markdown docs: %w", err)
		}

		fmt.Printf("Documentation generated in %s\n", outputDir)
		return nil
	},
}

var docsManCmd = &cobra.Command{
	Use:   "man [output-dir]",
	Short: "Generate man pages",
	Long: `Generate man page documentation for all CLI commands.

The generated files can be installed to /usr/share/man/man1/ for
system-wide availability.

Example:
  vcdeploy docs man /usr/share/man/man1/`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir := args[0]

		// Create output directory if it doesn't exist
		// #nosec G301 - Documentation output directory needs world-read access
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		// Generate man pages
		header := &doc.GenManHeader{
			Title:   "VCDEPLOY",
			Section: "1",
			Source:  "vcdeploy " + version,
		}

		err := doc.GenManTree(rootCmd, header, outputDir)
		if err != nil {
			return fmt.Errorf("generating man pages: %w", err)
		}

		fmt.Printf("Man pages generated in %s\n", outputDir)
		return nil
	},
}

var docsYamlCmd = &cobra.Command{
	Use:   "yaml [output-dir]",
	Short: "Generate YAML documentation",
	Long: `Generate YAML documentation for all CLI commands.

This format is useful for programmatic processing or integration
with documentation tools.

Example:
  vcdeploy docs yaml docs/reference/cli/yaml/`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir := args[0]

		// Create output directory if it doesn't exist
		// #nosec G301 - Documentation output directory needs world-read access
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		// Generate YAML documentation
		err := doc.GenYamlTree(rootCmd, outputDir)
		if err != nil {
			return fmt.Errorf("generating YAML docs: %w", err)
		}

		fmt.Printf("YAML documentation generated in %s\n", outputDir)
		return nil
	},
}

var docsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install man pages",
	Long: `Install man pages to the system or user man directory.

By default, installs to /usr/local/share/man/man1/ (requires sudo).
Use --user to install to ~/.local/share/man/man1/ instead.

Examples:
  sudo vcdeploy docs install       # System-wide install
  vcdeploy docs install --user     # User-local install`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userMode, _ := cmd.Flags().GetBool("user")

		var manDir string
		if userMode {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home directory: %w", err)
			}
			manDir = home + "/.local/share/man/man1"
		} else {
			manDir = "/usr/local/share/man/man1"
		}

		// Create output directory if it doesn't exist
		// #nosec G301 - Man page directory needs world-read access
		if err := os.MkdirAll(manDir, 0o755); err != nil {
			return fmt.Errorf("creating man directory: %w", err)
		}

		// Generate man pages
		header := &doc.GenManHeader{
			Title:   "VCDEPLOY",
			Section: "1",
			Source:  "vcdeploy " + version,
		}

		err := doc.GenManTree(rootCmd, header, manDir)
		if err != nil {
			return fmt.Errorf("generating man pages: %w", err)
		}

		fmt.Printf("Man pages installed to %s\n", manDir)
		if !userMode {
			fmt.Println("Run 'mandb' or 'makewhatis' to update the man page database.")
		}
		return nil
	},
}

func init() {
	docsInstallCmd.Flags().Bool("user", false, "Install to ~/.local/share/man (no sudo required)")
	docsCmd.AddCommand(docsMarkdownCmd)
	docsCmd.AddCommand(docsManCmd)
	docsCmd.AddCommand(docsYamlCmd)
	docsCmd.AddCommand(docsInstallCmd)
	rootCmd.AddCommand(docsCmd)
}
