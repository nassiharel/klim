package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/config"
)

var marketplaceCmd = &cobra.Command{
	Use:   "marketplace",
	Short: "Manage marketplace sources",
	Long: `Manage additional marketplace URLs. Extra marketplaces are merged
with the default catalog — tools from extra sources with the same name
as default tools will override them.

Configure in config.yaml:
  marketplace:
    extra_urls:
      - https://example.com/my-tools/marketplace.yaml
      - https://raw.githubusercontent.com/myorg/tools/main/marketplace.yaml`,
}

var marketplaceAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add an extra marketplace URL",
	Args:  requireArgs(1, "clim config marketplace add <url>"),
	RunE:  runMarketplaceAdd,
}

var marketplaceRemoveCmd = &cobra.Command{
	Use:   "remove <url>",
	Short: "Remove an extra marketplace URL",
	Args:  requireArgs(1, "clim config marketplace remove <url>"),
	RunE:  runMarketplaceRemove,
}

var marketplaceListOutputFmt func() (OutputFormat, error)

var marketplaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all marketplace URLs",
	RunE:  runMarketplaceList,
}

func init() {
	marketplaceListOutputFmt = addOutputFlag(marketplaceListCmd, OutputText, OutputJSON)
	marketplaceCmd.AddCommand(marketplaceAddCmd)
	marketplaceCmd.AddCommand(marketplaceRemoveCmd)
	marketplaceCmd.AddCommand(marketplaceListCmd)
	// Registered under configCmd in config.go.
}

func runMarketplaceAdd(cmd *cobra.Command, args []string) error {
	rawURL := strings.TrimSpace(args[0])
	if rawURL == "" {
		return errors.New("url cannot be empty")
	}

	// Validate URL scheme.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid URL %q: scheme must be http or https", rawURL)
	}
	if parsed.Host == "" {
		return fmt.Errorf("invalid URL %q: missing host", rawURL)
	}

	// Reload config fresh.
	c, _, err := config.LoadWithWarnings()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Normalize existing URLs and check for duplicates.
	var normalized []string
	for _, existing := range c.Marketplace.ExtraURLs {
		e := strings.TrimSpace(existing)
		if e == "" {
			continue
		}
		if e == rawURL {
			fmt.Fprintf(os.Stderr, "URL already configured: %s\n", rawURL)
			return nil
		}
		normalized = append(normalized, e)
	}

	normalized = append(normalized, rawURL)
	c.Marketplace.ExtraURLs = normalized

	if err := config.Save(c); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Added marketplace: %s\n", rawURL)
	fmt.Fprintf(os.Stderr, "  %d extra marketplace(s) configured. Run 'clim' to see merged tools.\n", len(c.Marketplace.ExtraURLs))
	return nil
}

func runMarketplaceRemove(cmd *cobra.Command, args []string) error {
	url := strings.TrimSpace(args[0])
	if url == "" {
		return errors.New("url cannot be empty")
	}

	c, _, err := config.LoadWithWarnings()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	found := false
	var filtered []string
	for _, existing := range c.Marketplace.ExtraURLs {
		e := strings.TrimSpace(existing)
		if e == url {
			found = true
			continue
		}
		if e != "" {
			filtered = append(filtered, e)
		}
	}

	if !found {
		fmt.Fprintf(os.Stderr, "URL not found: %s\n", url)
		return nil
	}

	c.Marketplace.ExtraURLs = filtered

	if err := config.Save(c); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Removed marketplace: %s\n", url)
	return nil
}

type marketplaceListReport struct {
	Primary string   `json:"primary"`
	Extra   []string `json:"extra"`
}

func runMarketplaceList(cmd *cobra.Command, args []string) error {
	out, err := marketplaceListOutputFmt()
	if err != nil {
		return err
	}

	c, _, loadErr := config.LoadWithWarnings()
	if loadErr != nil {
		// In JSON mode never fall back to synthetic data — automation has
		// no way to distinguish defaults from actual config otherwise.
		if out == OutputJSON {
			return fmt.Errorf("loading config: %w", loadErr)
		}
		fmt.Fprintf(os.Stderr, "⚠ Could not load config (%v); showing defaults.\n", loadErr)
		c = config.Default()
	}

	primaryURL := c.Marketplace.URL
	if primaryURL == "" {
		primaryURL = config.DefaultMarketplaceURL
	}

	if out == OutputJSON {
		extra := make([]string, 0, len(c.Marketplace.ExtraURLs))
		for _, u := range c.Marketplace.ExtraURLs {
			if e := strings.TrimSpace(u); e != "" {
				extra = append(extra, e)
			}
		}
		return printJSON(marketplaceListReport{Primary: primaryURL, Extra: extra})
	}

	fmt.Fprintf(os.Stderr, "Primary:\n  %s\n", primaryURL)

	if len(c.Marketplace.ExtraURLs) == 0 {
		fmt.Fprintln(os.Stderr, "\nNo extra marketplaces configured.")
		fmt.Fprintln(os.Stderr, "Add one with: clim config marketplace add <url>")
	} else {
		fmt.Fprintf(os.Stderr, "\nExtra (%d):\n", len(c.Marketplace.ExtraURLs))
		for i, url := range c.Marketplace.ExtraURLs {
			fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, url)
		}
	}

	return nil
}
