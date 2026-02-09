package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/tarrence/mercury-cli/internal/cligen"
	"github.com/tarrence/mercury-cli/internal/openapi"
)

type specSource struct {
	Name     string
	URL      string
	Filename string
}

var defaultSpecSources = []specSource{
	{
		Name:     "mwb",
		URL:      "https://dash.readme.com/api/v1/api-registry/3khi2p92mlbhngfx",
		Filename: "mwb-openapi.json",
	},
	{
		Name:     "onboarding",
		URL:      "https://dash.readme.com/api/v1/api-registry/3tcxm6d5mi3j2og0",
		Filename: "onboarding-openapi.json",
	},
	{
		Name:     "oauth2",
		URL:      "https://dash.readme.com/api/v1/api-registry/57ia0kcml8qkyms",
		Filename: "oauth2-openapi.json",
	},
}

func newSpecCmd(specDocs []*openapi.SpecDoc) *cobra.Command {
	specCmd := &cobra.Command{
		Use:           "spec",
		Short:         "OpenAPI spec utilities (for maintainers)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	specCmd.AddCommand(newSpecListCmd(specDocs))
	specCmd.AddCommand(newSpecVerifyCmd(specDocs))
	specCmd.AddCommand(newSpecUpdateCmd())

	return specCmd
}

func newSpecListCmd(specDocs []*openapi.SpecDoc) *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List embedded OpenAPI specs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, doc := range specDocs {
				if doc == nil || doc.Spec == nil {
					continue
				}
				ops := 0
				tags := map[string]bool{}
				for _, item := range doc.Spec.Paths {
					for _, op := range item.Operations() {
						if op == nil {
							continue
						}
						ops++
						for _, t := range op.Tags {
							tags[t] = true
						}
					}
				}
				tagList := make([]string, 0, len(tags))
				for t := range tags {
					tagList = append(tagList, t)
				}
				sort.Strings(tagList)

				server := doc.Spec.ServerURLForOperation(nil)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tops=%d\ttags=%d\tserver=%s\n", doc.Name, doc.Filename, ops, len(tagList), server)
			}
			return nil
		},
	}
}

func newSpecVerifyCmd(specDocs []*openapi.SpecDoc) *cobra.Command {
	return &cobra.Command{
		Use:           "verify",
		Short:         "Verify embedded OpenAPI specs are parseable and generate unique commands",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Basic JSON sanity: ensure we can re-marshal (catches some NaNs/invalid structures).
			for _, doc := range specDocs {
				if doc == nil || doc.Spec == nil {
					continue
				}
				if _, err := json.Marshal(doc.Spec); err != nil {
					return fmt.Errorf("spec %s marshal: %w", doc.Filename, err)
				}
			}

			// CLI generation sanity and uniqueness.
			root := &cobra.Command{Use: "verify-root"}
			if err := cligen.AddOpenAPICommands(root, specDocs); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
}

func newSpecUpdateCmd() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:           "update",
		Short:         "Download latest OpenAPI specs into the repo (updates specs/*.json)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if outDir == "" {
				outDir = "specs"
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}

			httpClient := &http.Client{Timeout: 30 * time.Second}
			for _, src := range defaultSpecSources {
				req, err := http.NewRequest(http.MethodGet, src.URL, nil)
				if err != nil {
					return err
				}
				resp, err := httpClient.Do(req)
				if err != nil {
					return err
				}
				b, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					return err
				}
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					return fmt.Errorf("download %s failed: %s", src.Name, resp.Status)
				}

				path := filepath.Join(outDir, src.Filename)
				if err := os.WriteFile(path, b, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out-dir", "specs", "Output directory for spec files")
	return cmd
}
