package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nonchord/kcompass/internal/backend"
)

// addOpts holds collected options for `kcompass operator add`.
type addOpts struct {
	provider    string
	name        string
	description string
	output      string
	// GKE
	project   string
	region    string
	clusterID string
	// EKS
	accountID   string
	clusterName string
	// Generic
	server string
	caData string
}

// NewOperatorAddCommand creates the `kcompass operator add` command.
//
// If all required flags are provided the command runs non-interactively, making
// it safe for scripts and CI. When required flags are omitted and stdin is a
// terminal, all fields (including optional ones) are collected via prompts.
func NewOperatorAddCommand() *cobra.Command {
	var opts addOpts
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a cluster entry to an inventory file",
		Long: `Add a cluster record to a kcompass inventory YAML file.

If all required flags are provided the command runs non-interactively (suitable for
scripts and CI pipelines). When required flags are omitted and stdin is a terminal,
all fields are collected via interactive prompts.

Output goes to stdout by default; use --output to append directly to a file.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdd(opts, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.provider, "provider", "p", "", "cluster provider: gke, eks, or generic")
	f.StringVarP(&opts.name, "name", "n", "", "cluster name")
	f.StringVarP(&opts.description, "description", "d", "", "human-readable description (optional)")
	f.StringVarP(&opts.output, "output", "o", "", "file to append to (default: stdout)")
	f.StringVar(&opts.project, "project", "", "GCP project ID (gke)")
	f.StringVar(&opts.region, "region", "", "region or zone (gke, eks)")
	f.StringVar(&opts.clusterID, "cluster-id", "", "GKE cluster ID (gke; defaults to --name)")
	f.StringVar(&opts.accountID, "account-id", "", "AWS account ID (eks)")
	f.StringVar(&opts.clusterName, "cluster-name", "", "EKS cluster name (eks; defaults to --name)")
	f.StringVar(&opts.server, "server", "", "API server URL (generic)")
	f.StringVar(&opts.caData, "ca-data", "", "base64-encoded CA certificate (generic, optional)")
	return cmd
}

func runAdd(opts addOpts, stdout io.Writer) error {
	// Interactive mode is only triggered when stdin is a terminal AND at least
	// one required field is missing. If all required fields are present via flags,
	// the command runs silently regardless of whether stdin is a terminal.
	interactive := stdinIsTerminal() && (opts.provider == "" || opts.name == "")

	// get returns current when non-empty. In interactive mode it prompts for
	// missing values; in non-interactive mode it returns the defaultVal silently
	// (callers validate emptiness for required fields).
	get := func(current, label, defaultVal string) (string, error) {
		if current != "" {
			return current, nil
		}
		if !interactive {
			return defaultVal, nil
		}
		return Prompt(os.Stdout, os.Stdin, label, defaultVal)
	}

	var err error

	opts.provider, err = get(opts.provider, "Provider (gke/eks/generic)", "")
	if err != nil {
		return err
	}
	opts.provider = strings.ToLower(strings.TrimSpace(opts.provider))
	switch opts.provider {
	case "gke", "eks", "generic":
	default:
		return fmt.Errorf("unknown provider %q: must be gke, eks, or generic", opts.provider)
	}

	opts.name, err = get(opts.name, "Cluster name", "")
	if err != nil {
		return err
	}
	if opts.name == "" {
		return fmt.Errorf("cluster name is required (use --name or run interactively)")
	}

	// Description is always asked in interactive mode; silently empty otherwise.
	opts.description, err = get(opts.description, "Description (optional)", "")
	if err != nil {
		return err
	}

	// Provider-specific fields.
	authMethod, metadata, err := collectProviderFields(opts, get)
	if err != nil {
		return err
	}

	rec := backend.ClusterRecord{
		Name:        opts.name,
		Description: opts.description,
		Provider:    opts.provider,
		Auth:        authMethod,
		Metadata:    metadata,
	}

	return writeInventoryRecord(rec, opts.output, stdout)
}

// getFunc is the signature of the collect/prompt helper passed to provider helpers.
type getFunc func(current, label, defaultVal string) (string, error)

// collectProviderFields gathers provider-specific metadata using get to
// prompt or use flags. Returns the auth method string and metadata map.
func collectProviderFields(opts addOpts, get getFunc) (string, map[string]string, error) {
	switch opts.provider {
	case "gke":
		return collectGKEFields(opts, get)
	case "eks":
		return collectEKSFields(opts, get)
	case "generic":
		return collectGenericFields(opts, get)
	}
	return "", nil, fmt.Errorf("unknown provider %q", opts.provider)
}

func collectGKEFields(opts addOpts, get getFunc) (string, map[string]string, error) {
	var err error
	opts.project, err = get(opts.project, "GCP project", "")
	if err != nil {
		return "", nil, err
	}
	opts.region, err = get(opts.region, "Region or zone", "")
	if err != nil {
		return "", nil, err
	}
	opts.clusterID, err = get(opts.clusterID, "GKE cluster ID", opts.name)
	if err != nil {
		return "", nil, err
	}
	if opts.project == "" || opts.region == "" || opts.clusterID == "" {
		return "", nil, fmt.Errorf("gke requires --project, --region, and --cluster-id")
	}
	return "gcloud", map[string]string{
		"project":    opts.project,
		"region":     opts.region,
		"cluster_id": opts.clusterID,
	}, nil
}

func collectEKSFields(opts addOpts, get getFunc) (string, map[string]string, error) {
	var err error
	opts.accountID, err = get(opts.accountID, "AWS account ID", "")
	if err != nil {
		return "", nil, err
	}
	opts.region, err = get(opts.region, "Region", "")
	if err != nil {
		return "", nil, err
	}
	opts.clusterName, err = get(opts.clusterName, "EKS cluster name", opts.name)
	if err != nil {
		return "", nil, err
	}
	if opts.accountID == "" || opts.region == "" || opts.clusterName == "" {
		return "", nil, fmt.Errorf("eks requires --account-id, --region, and --cluster-name")
	}
	return "aws", map[string]string{
		"account_id":   opts.accountID,
		"region":       opts.region,
		"cluster_name": opts.clusterName,
	}, nil
}

func collectGenericFields(opts addOpts, get getFunc) (string, map[string]string, error) {
	var err error
	opts.server, err = get(opts.server, "API server URL", "")
	if err != nil {
		return "", nil, err
	}
	if opts.server == "" {
		return "", nil, fmt.Errorf("generic requires --server")
	}
	opts.caData, err = get(opts.caData, "CA data (base64, optional)", "")
	if err != nil {
		return "", nil, err
	}
	meta := map[string]string{"server": opts.server}
	if opts.caData != "" {
		meta["ca_data"] = opts.caData
	}
	return "static", meta, nil
}

// writeInventoryRecord appends rec to outputPath, or emits a YAML snippet to stdout.
func writeInventoryRecord(rec backend.ClusterRecord, outputPath string, stdout io.Writer) error {
	if outputPath == "" {
		data, err := yaml.Marshal(inventoryFile{Clusters: []backend.ClusterRecord{rec}})
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		_, err = stdout.Write(data)
		return err
	}

	// Read the existing file or start empty.
	var cf inventoryFile
	data, err := os.ReadFile(outputPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", outputPath, err)
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &cf); err != nil {
			return fmt.Errorf("parse %s: %w", outputPath, err)
		}
	}

	cf.Clusters = append(cf.Clusters, rec)

	out, err := yaml.Marshal(cf)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(outputPath, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	_, _ = fmt.Fprintf(stdout, "Added %q to %s\n", rec.Name, outputPath)
	return nil
}

// inventoryFile is the on-disk structure for a cluster inventory YAML.
type inventoryFile struct {
	Clusters []backend.ClusterRecord `yaml:"clusters"`
}

// Prompt writes a prompt line to out and reads one line from in.
// If the user enters nothing, defaultVal is returned. Exported so tests can
// drive it with injected readers/writers.
func Prompt(out io.Writer, in io.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		_, _ = fmt.Fprintf(out, "%s [%s]: ", label, defaultVal)
	} else {
		_, _ = fmt.Fprintf(out, "%s: ", label)
	}
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return defaultVal, nil // EOF → accept default
	}
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

// stdinIsTerminal reports whether os.Stdin is an interactive terminal.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
