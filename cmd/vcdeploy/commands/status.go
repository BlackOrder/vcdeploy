// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// statsCmd displays system stats
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Display system stats",
	Long: `Display system stats with key metrics.

Shows total projects, agents, deployments, and success rate.
Use --watch to continuously update the display.

Subcommands:
  deployments  Deployment-focused analytics
  agents       Agent health statistics`,
	Example: `  # Show combined stats
  vcdeploy stats

  # Watch stats updates every 5 seconds
  vcdeploy stats --watch

  # Watch with custom interval
  vcdeploy stats --watch --interval 10

  # Deployment analytics only
  vcdeploy stats deployments

  # Agent health only
  vcdeploy stats agents`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statsCmd)

	statsCmd.Flags().BoolP("watch", "w", false, "Continuously update display")
	statsCmd.Flags().IntP("interval", "i", 5, "Update interval in seconds (with --watch)")
	statsCmd.Flags().StringP("output", "o", "table", "Output format: table, json")

	// Subcommands
	statsCmd.AddCommand(&cobra.Command{
		Use:   "deployments",
		Short: "Deployment-focused analytics",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient(cmd)
			if err != nil {
				return err
			}
			status, err := fetchStatus(client)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "🚀 DEPLOYMENT ANALYTICS (24h)")
			fmt.Fprintf(w, "   Total:\t%d\n", status.Deployments.Total)
			fmt.Fprintf(w, "   Successful:\t%d ✓\n", status.Deployments.Successful)
			fmt.Fprintf(w, "   Failed:\t%d ✗\n", status.Deployments.Failed)
			if status.Deployments.Running > 0 {
				fmt.Fprintf(w, "   Running:\t%d ⟳\n", status.Deployments.Running)
			}
			if status.Deployments.Pending > 0 {
				fmt.Fprintf(w, "   Pending:\t%d ○\n", status.Deployments.Pending)
			}
			if status.Deployments.Total > 0 {
				fmt.Fprintf(w, "   Success Rate:\t%.1f%%\n", status.Deployments.SuccessRate)
			}
			return w.Flush()
		},
	})

	statsCmd.AddCommand(&cobra.Command{
		Use:   "agents",
		Short: "Agent health statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient(cmd)
			if err != nil {
				return err
			}
			status, err := fetchStatus(client)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "🖥️  AGENT HEALTH")
			fmt.Fprintf(w, "   Total:\t%d\n", status.Agents.Total)
			fmt.Fprintf(w, "   Online:\t%d ✓\n", status.Agents.Online)
			fmt.Fprintf(w, "   Offline:\t%d ✗\n", status.Agents.Offline)
			return w.Flush()
		},
	})
}

// statusResponse represents the aggregated status data
type statusResponse struct {
	Projects    projectStats    `json:"projects"`
	Agents      agentStats      `json:"agents"`
	Deployments deploymentStats `json:"deployments"`
	ServerTime  time.Time       `json:"server_time"`
}

type projectStats struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

type agentStats struct {
	Total   int `json:"total"`
	Online  int `json:"online"`
	Offline int `json:"offline"`
}

type deploymentStats struct {
	Total       int     `json:"total"`
	Successful  int     `json:"successful"`
	Failed      int     `json:"failed"`
	Pending     int     `json:"pending"`
	Running     int     `json:"running"`
	SuccessRate float64 `json:"success_rate"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	watch, _ := cmd.Flags().GetBool("watch")
	interval, _ := cmd.Flags().GetInt("interval")
	output, _ := cmd.Flags().GetString("output")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	if watch {
		return watchStatus(client, interval, output)
	}

	return printStatus(client, output)
}

func printStatus(client *apiClient, format string) error {
	status, err := fetchStatus(client)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	default:
		return printStatusTable(status)
	}
}

func watchStatus(client *apiClient, interval int, format string) error {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// Clear screen and print first status
	fmt.Print("\033[2J\033[H") // ANSI clear screen
	if err := printStatus(client, format); err != nil {
		return err
	}

	for range ticker.C {
		fmt.Print("\033[2J\033[H") // Clear screen
		if err := printStatus(client, format); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	return nil
}

func fetchStatus(client *apiClient) (*statusResponse, error) {
	status := &statusResponse{
		ServerTime: time.Now(),
	}

	// Fetch projects count
	if resp, err := client.get("/api/v1/projects?limit=0"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var result struct {
				Total int `json:"total"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
				status.Projects.Total = result.Total
				status.Projects.Active = result.Total // Assume all active for now
			}
		}
	}

	// Fetch agents
	if resp, err := client.get("/api/v1/agents"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var agents []struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&agents); err == nil {
				status.Agents.Total = len(agents)
				for _, a := range agents {
					if a.Status == "online" {
						status.Agents.Online++
					} else {
						status.Agents.Offline++
					}
				}
			}
		}
	}

	// Fetch deployment stats
	if resp, err := client.get("/api/v1/stats/deployments?range=24h"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var stats deploymentStats
			if err := json.NewDecoder(resp.Body).Decode(&stats); err == nil {
				status.Deployments = stats
			}
		}
	}

	return status, nil
}

func printStatusTable(status *statusResponse) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "╭─────────────────────────────────────────╮")
	fmt.Fprintln(w, "│         VCDEPLOY SYSTEM STATUS          │")
	fmt.Fprintln(w, "╰─────────────────────────────────────────╯")
	fmt.Fprintln(w)

	// Projects section
	fmt.Fprintln(w, "📦 PROJECTS")
	fmt.Fprintf(w, "   Total:\t%d\n", status.Projects.Total)
	fmt.Fprintln(w)

	// Agents section
	fmt.Fprintln(w, "🖥️  AGENTS")
	fmt.Fprintf(w, "   Total:\t%d\n", status.Agents.Total)
	fmt.Fprintf(w, "   Online:\t%d ✓\n", status.Agents.Online)
	fmt.Fprintf(w, "   Offline:\t%d ✗\n", status.Agents.Offline)
	fmt.Fprintln(w)

	// Deployments section
	fmt.Fprintln(w, "🚀 DEPLOYMENTS (24h)")
	fmt.Fprintf(w, "   Total:\t%d\n", status.Deployments.Total)
	fmt.Fprintf(w, "   Successful:\t%d ✓\n", status.Deployments.Successful)
	fmt.Fprintf(w, "   Failed:\t%d ✗\n", status.Deployments.Failed)
	if status.Deployments.Running > 0 {
		fmt.Fprintf(w, "   Running:\t%d ⟳\n", status.Deployments.Running)
	}
	if status.Deployments.Pending > 0 {
		fmt.Fprintf(w, "   Pending:\t%d ○\n", status.Deployments.Pending)
	}
	if status.Deployments.Total > 0 {
		fmt.Fprintf(w, "   Success Rate:\t%.1f%%\n", status.Deployments.SuccessRate)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Last updated: %s\n", status.ServerTime.Format("2006-01-02 15:04:05"))

	return w.Flush()
}
