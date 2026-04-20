package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/micaelmalta/loi/internal/claims"
	"github.com/micaelmalta/loi/internal/git"
	"github.com/spf13/cobra"
)

// ── loi claim ────────────────────────────────────────────────────────────────

var (
	claimIntent string
	claimTTL    string
)

var claimCmd = &cobra.Command{
	Use:   "claim <room>",
	Short: "Claim a LOI room with an intent",
	Long: `Claim a LOI room so other agents know you are working in it.

The claim is advisory: it writes an entry to .loi-claims.json in the
repository root.  Conflicts are reported but only blocking-intent pairs
(edit vs edit) cause a non-zero exit.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		room := args[0]

		ttl, err := claims.ParseTTL(claimTTL)
		if err != nil {
			return fmt.Errorf("invalid --ttl %q: %w", claimTTL, err)
		}

		store := claims.NewClaimsStore(projectRoot)
		existing, err := store.GetClaimsFor(room)
		if err != nil {
			return fmt.Errorf("reading claims: %w", err)
		}

		action, msg := claims.CheckConflict(existing, claimIntent)
		switch action {
		case claims.ActionConflict:
			fmt.Fprintln(os.Stderr, msg)
			os.Exit(1)
		case claims.ActionAllowWithWarning, claims.ActionAllowWithVisibility, claims.ActionGovernanceSensitive:
			fmt.Fprintln(os.Stderr, msg)
		}

		agentID := claims.AgentID()
		sessionID := claims.SessionID(agentID)
		now := time.Now().UTC()

		c := claims.Claim{
			ScopeType: "room",
			ScopeID:   room,
			Repo:      git.RepoName(projectRoot),
			AgentID:   agentID,
			SessionID: sessionID,
			Intent:    claimIntent,
			ClaimedAt: now,
			ExpiresAt: now.Add(ttl),
			Branch:    git.Branch(projectRoot),
		}

		if err := store.AddClaim(c); err != nil {
			return fmt.Errorf("storing claim: %w", err)
		}

		fmt.Printf("Claimed '%s' with intent=%s, ttl=%s\n", room, claimIntent, claimTTL)
		fmt.Printf("  agent   : %s\n", agentID)
		fmt.Printf("  branch  : %s\n", c.Branch)
		fmt.Printf("  expires : %s\n", c.ExpiresAt.Format(time.RFC3339))
		return nil
	},
}

// ── loi heartbeat ────────────────────────────────────────────────────────────

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat <room>",
	Short: "Extend the expiry of an existing claim",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		room := args[0]
		agentID := claims.AgentID()

		store := claims.NewClaimsStore(projectRoot)
		updated, err := store.UpdateExpiry(room, agentID, claims.HeartbeatGrace)
		if err != nil {
			return fmt.Errorf("updating expiry: %w", err)
		}
		if !updated {
			fmt.Fprintf(os.Stderr, "no active claim found for room=%s agent=%s\n", room, agentID)
			os.Exit(1)
		}
		fmt.Printf("Heartbeat recorded: room=%s agent=%s grace=%s\n",
			room, agentID, claims.HeartbeatGrace)
		return nil
	},
}

// ── loi release ──────────────────────────────────────────────────────────────

var releaseCmd = &cobra.Command{
	Use:   "release <room>",
	Short: "Release a previously claimed room",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		room := args[0]
		agentID := claims.AgentID()

		store := claims.NewClaimsStore(projectRoot)
		removed, err := store.RemoveClaim(room, agentID)
		if err != nil {
			return fmt.Errorf("removing claim: %w", err)
		}
		if removed {
			fmt.Printf("Released claim: room=%s agent=%s\n", room, agentID)
		} else {
			fmt.Printf("No active claim found for room=%s agent=%s (nothing to release)\n", room, agentID)
		}
		return nil
	},
}

// ── loi status ───────────────────────────────────────────────────────────────

var statusIncludeFreshness bool

var statusCmd = &cobra.Command{
	Use:   "status <room>",
	Short: "Show active claims and summaries for a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		room := args[0]
		store := claims.NewClaimsStore(projectRoot)

		roomClaims, err := store.GetClaimsFor(room)
		if err != nil {
			return fmt.Errorf("reading claims: %w", err)
		}
		summaries, err := store.GetSummariesFor(room)
		if err != nil {
			return fmt.Errorf("reading summaries: %w", err)
		}

		fmt.Printf("Room: %s\n\n", room)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if len(roomClaims) == 0 {
			fmt.Println("No active claims.")
		} else {
			fmt.Fprintln(w, "AGENT\tINTENT\tBRANCH\tEXPIRES")
			for _, c := range roomClaims {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					c.AgentID, c.Intent, c.Branch,
					c.ExpiresAt.Format(time.RFC3339))
			}
			w.Flush()
		}

		if statusIncludeFreshness && len(summaries) > 0 {
			// Print last 5 summaries.
			start := max(0, len(summaries)-5)
			recent := summaries[start:]
			fmt.Printf("\nRecent summaries (%d):\n", len(recent))
			for _, s := range recent {
				fmt.Printf("  [%s] %s: %s\n",
					s.RecordedAt.Format("2006-01-02 15:04"), s.AgentID, s.Text)
			}
		}
		return nil
	},
}

// ── loi summary ──────────────────────────────────────────────────────────────

var summaryCmd = &cobra.Command{
	Use:   "summary <room> <text>",
	Short: "Publish a work summary for a room",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		room := args[0]
		text := args[1]
		agentID := claims.AgentID()

		store := claims.NewClaimsStore(projectRoot)
		if err := store.AddSummary(room, agentID, text); err != nil {
			return fmt.Errorf("adding summary: %w", err)
		}
		fmt.Printf("Summary recorded: room=%s agent=%s\n", room, agentID)
		return nil
	},
}

// ── loi claims ───────────────────────────────────────────────────────────────

var claimsFilterRepo string

var claimsCmd = &cobra.Command{
	Use:   "claims",
	Short: "List all active claims across the store",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := claims.NewClaimsStore(projectRoot)
		all, err := store.AllClaims()
		if err != nil {
			return fmt.Errorf("reading claims: %w", err)
		}

		if claimsFilterRepo != "" {
			filtered := all[:0]
			for _, c := range all {
				if c.Repo == claimsFilterRepo {
					filtered = append(filtered, c)
				}
			}
			all = filtered
		}

		if len(all) == 0 {
			fmt.Println("No active claims.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ROOM\tAGENT\tINTENT\tBRANCH\tEXPIRES")
		for _, c := range all {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				c.ScopeID, c.AgentID, c.Intent, c.Branch,
				c.ExpiresAt.Format(time.RFC3339))
		}
		return w.Flush()
	},
}

func init() {
	// claim
	claimCmd.Flags().StringVar(&claimIntent, "intent", "edit",
		"Intent for the claim: edit, read, review, or security-sweep")
	claimCmd.Flags().StringVar(&claimTTL, "ttl", "15m",
		"Time-to-live for the claim (e.g. 15m, 2h, 1d)")
	rootCmd.AddCommand(claimCmd)

	// heartbeat
	rootCmd.AddCommand(heartbeatCmd)

	// release
	rootCmd.AddCommand(releaseCmd)

	// status
	statusCmd.Flags().BoolVar(&statusIncludeFreshness, "include-freshness", false,
		"Print the last 5 room summaries alongside claims")
	rootCmd.AddCommand(statusCmd)

	// summary
	rootCmd.AddCommand(summaryCmd)

	// claims
	claimsCmd.Flags().StringVar(&claimsFilterRepo, "repo", "",
		"Filter claims by repository name")
	rootCmd.AddCommand(claimsCmd)
}
