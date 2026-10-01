package cli

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type app struct {
	in                        io.Reader
	out, errOut               io.Writer
	getenv                    func(string) string
	userConfigDir             func() (string, error)
	keys                      keyStore
	transport                 http.RoundTripper
	version, profile, baseURL string
	compact                   bool
	timeout                   time.Duration
	retries                   int
	credentials               *credentials
	metadata                  *commandTransport
	started, mutation         bool
}

// Run executes one invocation without prompting or reading application .env files.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, version string) int {
	a := &app{in: in, out: out, errOut: errOut, getenv: os.Getenv,
		userConfigDir: os.UserConfigDir, keys: systemKeyStore{}, version: version}
	return a.execute(ctx, args)
}

func (a *app) execute(ctx context.Context, args []string) int {
	a.credentials, a.metadata, a.started, a.mutation = nil, nil, false, false
	cmd := a.command()
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	if !a.started {
		err = usageError(err.Error())
	}
	return a.writeError(err)
}

func (a *app) command() *cobra.Command {
	root := &cobra.Command{
		Use: "dnscale", Short: "Manage DNScale DNS from the terminal",
		Long:    "DNScale CLI: zones, records, DNSSEC inspection, and usage.\nResults are JSON. Use --json for compact output and --help on any command.",
		Version: a.version, SilenceUsage: true, SilenceErrors: true,
		PersistentPreRun: func(_ *cobra.Command, _ []string) { a.started = true },
		Args:             noArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.SetIn(a.in)
	root.SetOut(a.out)
	root.SetErr(a.errOut)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err.Error()) })
	f := root.PersistentFlags()
	f.StringVar(&a.profile, "profile", "", "Named keychain profile (overrides ambient API key/base URL)")
	f.StringVar(&a.baseURL, "base-url", "", "API endpoint including /v1 (HTTPS, or loopback HTTP)")
	f.BoolVar(&a.compact, "json", false, "Emit compact JSON instead of indented JSON")
	f.DurationVar(&a.timeout, "timeout", 30*time.Second, "Total API command timeout, including resolution, pagination, and retries")
	f.IntVar(&a.retries, "retries", 2, "Maximum retries for reads (0–10); mutations are never retried")
	root.AddCommand(a.authCommand())
	root.AddCommand(a.zonesCommand(), a.recordsCommand(), a.dnssecCommand(), a.usageCommand())
	return root
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return usageError(err.Error())
	}
	return nil
}

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(n)(cmd, args); err != nil {
			return usageError(err.Error())
		}
		return nil
	}
}

type commandError struct {
	code, message string
	exit          int
}

func (e *commandError) Error() string        { return e.message }
func usageError(message string) error        { return &commandError{"usage_error", message, 2} }
func localError(message string) error        { return &commandError{"configuration_error", message, 2} }
func resultError(code, message string) error { return &commandError{code, message, 1} }
