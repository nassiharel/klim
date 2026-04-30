package cli

import (
	"fmt"
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
	Args:  cobra.ExactArgs(1),
	RunE:  runMarketplaceAdd,
}

var marketplaceRemoveCmd = &cobra.Command{
	Use:   "remove <url>",
	Short: "Remove an extra marketplace URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runMarketplaceRemove,
}

var marketplaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all marketplace URLs",
	RunE:  runMarketplaceList,
}

func init() {
	marketplaceCmd.AddCommand(marketplaceAddCmd)
	marketplaceCmd.AddCommand(marketplaceRemoveCmd)
	marketplaceCmd.AddCommand(marketplaceListCmd)
	rootCmd.AddCommand(marketplaceCmd)
}

func runMarketplaceAdd(cmd *cobra.Command, args []string) error {
	url := strings.TrimSpace(args[0])
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// Reload config fresh.
	c, _, err := config.LoadWithWarnings()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Check for duplicates.
	for _, existing := range c.Marketplace.ExtraURLs {
		if existing == url {
			fmt.Fprintf(os.Stderr, "URL already configured: %s\n", url)
			return nil
		}
	}

	c.Marketplace.ExtraURLs = append(c.Marketplace.ExtraURLs, url)

	if err := config.Save(c); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Added marketplace: %s\n", url)
	fmt.Fprintf(os.Stderr, "  %d extra marketplace(s) configured. Run 'clim' to see merged tools.\n", len(c.Marketplace.ExtraURLs))
	return nil
}

func runMarketplaceRemove(cmd *cobra.Command, args []string) error {
	url := strings.TrimSpace(args[0])
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	c, _, err := config.LoadWithWarnings()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	found := false
	var filtered []string
	for _, existing := range c.Marketplace.ExtraURLs {
		if existing == url {
			found = true
			continue
		}
		filtered = append(filtered, existing)
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

func runMarketplaceList(cmd *cobra.Command, args []string) error {
	c, _, err := config.LoadWithWarnings()
	if err != nil {
		c = config.Default()
	}

	primaryURL := c.Marketplace.URL
	if primaryURL == "" {
		primaryURL = config.DefaultMarketplaceURL
	}

	fmt.Fprintf(os.Stderr, "Primary:\n  %s\n", primaryURL)

	if len(c.Marketplace.ExtraURLs) == 0 {
		fmt.Fprintln(os.Stderr, "\nNo extra marketplaces configured.")
		fmt.Fprintln(os.Stderr, "Add one with: clim marketplace add <url>")
	} else {
		fmt.Fprintf(os.Stderr, "\nExtra (%d):\n", len(c.Marketplace.ExtraURLs))
		for i, url := range c.Marketplace.ExtraURLs {
			fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, url)
		}
	}

	return nil
}
