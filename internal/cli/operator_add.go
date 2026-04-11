package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/nonchord/kcompass/internal/backend"
)

// addOpts holds collected options for `kcompass operator add`.
type addOpts struct {
	name        string
	description string
	output      string
	// Exactly one of these must be set.
	command        string // whitespace-separated argv string
	kubeconfigPath string // path to a kubeconfig file to embed inline
}

// NewOperatorAddCommand creates the `kcompass operator add` command.
//
// If --name and one of (--command, --kubeconfig) are present the command runs
// non-interactively. When required values are missing and stdin is a terminal,
// missing fields are prompted for.
func NewOperatorAddCommand() *cobra.Command {
	var opts addOpts
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a cluster entry to an inventory file",
		Long: `Add a cluster record to a kcompass inventory YAML file.

Each cluster needs either:
  --command "<argv>"        a command kcompass runs to mint a per-user kubeconfig
                            (e.g. "tailscale configure kubeconfig my-cluster" or
                             "gcloud container clusters get-credentials prod ...")
  --kubeconfig PATH         a kubeconfig file to embed inline in the record
                            (use this when the same kubeconfig works for everyone)

If required flags are omitted and stdin is a terminal, missing values are
collected via interactive prompts. Output goes to stdout by default; use
--output to append directly to a file.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdd(opts, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.name, "name", "n", "", "cluster name")
	f.StringVarP(&opts.description, "description", "d", "", "human-readable description (optional)")
	f.StringVarP(&opts.output, "output", "o", "", "file to append to (default: stdout)")
	f.StringVar(&opts.command, "command", "", `argv to run for credential acquisition, e.g. "tailscale configure kubeconfig staging"`)
	f.StringVar(&opts.kubeconfigPath, "kubeconfig", "", "path to a kubeconfig file to embed inline")
	return cmd
}

func runAdd(opts addOpts, stdout io.Writer) error {
	// Interactive mode triggers when stdin is a terminal AND something required
	// is missing. With all required flags present, the command stays silent.
	missingRequired := opts.name == "" || (opts.command == "" && opts.kubeconfigPath == "")
	interactive := stdinIsTerminal() && missingRequired

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
	opts.name, err = get(opts.name, "Cluster name", "")
	if err != nil {
		return err
	}
	if opts.name == "" {
		return errors.New("cluster name is required (use --name or run interactively)")
	}

	opts.description, err = get(opts.description, "Description (optional)", "")
	if err != nil {
		return err
	}

	if err := resolveCredSource(&opts, interactive); err != nil {
		return err
	}

	spec, err := buildKubeconfigSpec(opts.command, opts.kubeconfigPath)
	if err != nil {
		return err
	}

	rec := backend.ClusterRecord{
		Name:        opts.name,
		Description: opts.description,
		Kubeconfig:  spec,
	}
	if err := rec.Validate(); err != nil {
		return err
	}
	return writeInventoryRecord(rec, opts.output, stdout)
}

// resolveCredSource validates the command/kubeconfig flag combo and, when
// running interactively, prompts the user to choose a mode and supply the value.
func resolveCredSource(opts *addOpts, interactive bool) error {
	if opts.command != "" && opts.kubeconfigPath != "" {
		return errors.New("--command and --kubeconfig are mutually exclusive")
	}
	if opts.command != "" || opts.kubeconfigPath != "" {
		return nil
	}
	if !interactive {
		return errors.New("must provide either --command or --kubeconfig")
	}
	choice, err := Prompt(os.Stdout, os.Stdin,
		"Credential mode: (1) command  (2) kubeconfig file", "1")
	if err != nil {
		return err
	}
	switch strings.TrimSpace(choice) {
	case "1", "command":
		opts.command, err = Prompt(os.Stdout, os.Stdin,
			"Command (e.g. tailscale configure kubeconfig my-cluster)", "")
	case "2", "kubeconfig", "file":
		opts.kubeconfigPath, err = Prompt(os.Stdout, os.Stdin,
			"Path to kubeconfig file", "")
	default:
		return fmt.Errorf("unknown choice %q", choice)
	}
	return err
}

// buildKubeconfigSpec produces a KubeconfigSpec from one of the two flag inputs.
// Exactly one of command or path must be non-empty (caller enforces).
func buildKubeconfigSpec(command, path string) (backend.KubeconfigSpec, error) {
	if command != "" {
		argv := strings.Fields(command)
		if len(argv) == 0 {
			return backend.KubeconfigSpec{}, errors.New("--command is empty")
		}
		return backend.KubeconfigSpec{Command: argv}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return backend.KubeconfigSpec{}, fmt.Errorf("read kubeconfig %s: %w", path, err)
	}
	if len(data) == 0 {
		return backend.KubeconfigSpec{}, fmt.Errorf("kubeconfig file %s is empty", path)
	}
	return backend.KubeconfigSpec{Inline: string(data)}, nil
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
// It uses golang.org/x/term so a redirect to /dev/null in tests is correctly
// detected as non-interactive (a plain ModeCharDevice check would return true
// for /dev/null since it is itself a character device).
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
