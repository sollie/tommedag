// Command tommedag-cli resolves a street address in the Avfall Sør region
// to its waste pickup schedule and prints it as JSON.
//
// Usage:
//
//	tommedag-cli [flags] <address...>
//	tommedag-cli Dronningens gate 33
//	tommedag-cli -compact "Vågsbygdveien 21"
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sollie/tommedag/internal/avfallsor"
	"github.com/sollie/tommedag/internal/pickup"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entrypoint. It returns the process exit code and writes
// JSON output to stdout and human-readable diagnostics to stderr.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tommedag-cli", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: tommedag-cli [flags] <address...>\n\n"+
			"Resolves a street address to its Avfall Sør waste pickup schedule\n"+
			"and prints it as JSON.\n\nFlags:\n")
		fs.PrintDefaults()
		_, _ = fmt.Fprint(stderr, "\nExample:\n  tommedag-cli Dronningens gate 33\n")
	}

	timeout := fs.Duration("timeout", 10*time.Second, "upstream request timeout")
	compact := fs.Bool("compact", false, "emit compact single-line JSON instead of indented")
	baseURL := fs.String("base-url", "", "override upstream base URL (advanced/testing)")

	if err := fs.Parse(args); err != nil {
		// flag already printed the error and usage.
		return exitUsage
	}

	address := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if address == "" {
		_, _ = fmt.Fprintln(stderr, "error: an address argument is required")
		fs.Usage()
		return exitUsage
	}

	opts := []avfallsor.Option{
		// A one-shot process gets no benefit from the calendar cache.
		avfallsor.WithCacheTTL(0),
		avfallsor.WithHTTPClient(&http.Client{Timeout: *timeout}),
	}
	if *baseURL != "" {
		opts = append(opts, avfallsor.WithBaseURL(*baseURL))
	}
	client := avfallsor.NewClient(opts...)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	resp, err := resolve(ctx, client, address)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", userError(err))
		return exitError
	}

	enc := json.NewEncoder(stdout)
	if !*compact {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(resp); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: encoding output: %v\n", err)
		return exitError
	}

	return exitOK
}

// resolve runs the two-step lookup: address -> property match -> calendar.
func resolve(ctx context.Context, client *avfallsor.Client, address string) (pickup.Response, error) {
	match, err := client.ResolveAddress(ctx, address)
	if err != nil {
		return pickup.Response{}, err
	}

	cal, err := client.PickupCalendar(ctx, match.PropertyID)
	if err != nil {
		return pickup.Response{}, err
	}

	return pickup.NewResponse(address, match, cal), nil
}

// userError maps internal sentinel errors to friendly CLI messages.
func userError(err error) string {
	switch {
	case errors.Is(err, avfallsor.ErrAddressNotFound):
		return "no matching address found"
	case errors.Is(err, avfallsor.ErrInvalidPropertyID):
		return "no matching address found"
	case errors.Is(err, avfallsor.ErrUpstreamUnavailable):
		return "avfallsor.no is unavailable or returned an unexpected response"
	default:
		return err.Error()
	}
}
