// Package cli implements the tailapp command surface as a client of the
// resident control socket, apart from init/serve process lifecycle and MCP.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/generalbusiness-ai/tailapp/internal/control"
	"github.com/generalbusiness-ai/tailapp/internal/engine"
	"github.com/generalbusiness-ai/tailapp/internal/ingest"
	"github.com/generalbusiness-ai/tailapp/internal/mcp"
)

func Home() (string, error) {
	if value := os.Getenv("TAILAPP_HOME"); value != "" {
		return filepath.Abs(value)
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "tailapp"), nil
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usage(stderr)
	}
	home, err := Home()
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		if err := engine.InitHome(home); err != nil {
			return err
		}
		return output(stdout, map[string]any{"home": home})
	case "serve":
		return serve(ctx, home, args[1:], stdout, stderr)
	case "mcp":
		client := control.NewClient(filepath.Join(home, "engine.sock"))
		return (&mcp.Server{Client: client}).Serve(ctx, os.Stdin, os.Stdout)
	case "health":
		return call(stdout, home, "health", nil)
	case "apps":
		return apps(stdout, home, args[1:])
	case "query":
		return queryCommand(stdout, home, args[1:])
	case "ingest-fixture":
		return ingestFixture(ctx, stdout, home, args[1:])
	default:
		return usage(stderr)
	}
}

func usage(writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "usage: tailapp <init|serve|health|apps|query|ingest-fixture|mcp>")
	return errors.New("invalid command")
}

func serve(ctx context.Context, home string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	otlp := flags.String("otlp-http", "127.0.0.1:4318", "loopback OTLP/HTTP address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := ingest.ValidateLoopbackAddress(*otlp); err != nil {
		return err
	}
	resident, err := engine.Open(ctx, home)
	if err != nil {
		return err
	}
	defer resident.Close()
	socket := filepath.Join(home, "engine.sock")
	listener, err := control.Listen(socket)
	if err != nil {
		return err
	}
	defer func() { listener.Close(); _ = os.Remove(socket) }()
	controlServer := &http.Server{Handler: &control.Server{Engine: resident}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	otlpListener, err := net.Listen("tcp", *otlp)
	if err != nil {
		return err
	}
	defer otlpListener.Close()
	actualOTLP := otlpListener.Addr().String()
	otlpServer := &http.Server{Handler: resident.Receiver(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	metadata, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "started_at": time.Now().UTC().Format(time.RFC3339Nano), "otlp_http": "http://" + actualOTLP, "profile": statusProfile(resident, ctx), "version": "0.1.0"})
	if err := os.WriteFile(filepath.Join(home, "engine.json"), metadata, 0o600); err != nil {
		return err
	}
	errorsChannel := make(chan error, 2)
	go func() {
		err := controlServer.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			errorsChannel <- err
		}
	}()
	go func() {
		err := otlpServer.Serve(otlpListener)
		if !errors.Is(err, http.ErrServerClosed) {
			errorsChannel <- err
		}
	}()
	_ = output(stdout, map[string]any{"home": home, "control_socket": socket, "otlp_http": "http://" + actualOTLP})
	stopCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errorsChannel:
		return err
	case <-stopCtx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = otlpServer.Shutdown(shutdown)
		_ = controlServer.Shutdown(shutdown)
		return nil
	}
}
func statusProfile(resident *engine.Engine, ctx context.Context) string {
	status, err := resident.Status(ctx)
	if err != nil {
		return "unknown"
	}
	return status.Profile
}

func apps(stdout io.Writer, home string, args []string) error {
	if len(args) == 0 {
		return errors.New("apps subcommand required")
	}
	client := control.NewClient(filepath.Join(home, "engine.sock"))
	ctx := context.Background()
	var result any
	switch args[0] {
	case "list":
		return clientCallOutput(ctx, stdout, client, "apps_list", nil)
	case "create":
		flags := flag.NewFlagSet("apps create", flag.ContinueOnError)
		bundle := flags.String("bundle", "", "bundled source name")
		idempotencyKey := flags.String("idempotency-key", "", "stable retry key")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 || *idempotencyKey == "" {
			return errors.New("apps create requires NAME --idempotency-key KEY")
		}
		return clientCallOutput(ctx, stdout, client, "app_create", control.CreateArgs{Name: flags.Arg(0), Bundle: *bundle, IdempotencyKey: *idempotencyKey})
	case "get":
		if len(args) != 2 {
			return errors.New("apps get requires NAME")
		}
		return clientCallOutput(ctx, stdout, client, "app_get", control.NameArgs{Name: args[1]})
	case "delete":
		flags := flag.NewFlagSet("apps delete", flag.ContinueOnError)
		idempotencyKey := flags.String("idempotency-key", "", "stable retry key")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 || *idempotencyKey == "" {
			return errors.New("apps delete requires NAME --idempotency-key KEY")
		}
		return clientCallOutput(ctx, stdout, client, "app_delete", control.DeleteArgs{Name: flags.Arg(0), IdempotencyKey: *idempotencyKey})
	case "put":
		flags := flag.NewFlagSet("apps put", flag.ContinueOnError)
		expected := flags.String("expected", "", "expected draft revision")
		idempotencyKey := flags.String("idempotency-key", "", "stable retry key")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 3 || *idempotencyKey == "" {
			return errors.New("apps put requires NAME PATH FILE --idempotency-key KEY")
		}
		content, err := os.ReadFile(flags.Arg(2))
		if err != nil {
			return err
		}
		return clientCallOutput(ctx, stdout, client, "element_put", control.PutArgs{Name: flags.Arg(0), Path: flags.Arg(1), Content: content, ExpectedRevision: *expected, IdempotencyKey: *idempotencyKey})
	case "rm":
		flags := flag.NewFlagSet("apps rm", flag.ContinueOnError)
		expected := flags.String("expected", "", "expected draft revision")
		idempotencyKey := flags.String("idempotency-key", "", "stable retry key")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 2 || *idempotencyKey == "" {
			return errors.New("apps rm requires NAME PATH --idempotency-key KEY")
		}
		return clientCallOutput(ctx, stdout, client, "element_delete", control.RemoveArgs{Name: flags.Arg(0), Path: flags.Arg(1), ExpectedRevision: *expected, IdempotencyKey: *idempotencyKey})
	case "validate":
		flags := flag.NewFlagSet("apps validate", flag.ContinueOnError)
		expected := flags.String("expected", "", "expected draft revision")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("apps validate requires NAME")
		}
		return clientCallOutput(ctx, stdout, client, "validate", control.ValidateArgs{Name: flags.Arg(0), ExpectedRevision: *expected})
	case "activate":
		flags := flag.NewFlagSet("apps activate", flag.ContinueOnError)
		expected := flags.String("expected", "", "expected draft revision")
		mode := flags.String("mode", "continue", "continue or reset")
		ack := flags.Bool("ack-reset", false, "acknowledge reset data loss")
		idempotencyKey := flags.String("idempotency-key", "", "stable retry key")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 || *idempotencyKey == "" {
			return errors.New("apps activate requires NAME --idempotency-key KEY")
		}
		return clientCallOutput(ctx, stdout, client, "activate", control.ActivateArgs{Name: flags.Arg(0), ExpectedRevision: *expected, Mode: *mode, AcknowledgeReset: *ack, IdempotencyKey: *idempotencyKey})
	case "status":
		return clientCallOutput(ctx, stdout, client, "status", nil)
	case "schema":
		if len(args) != 2 {
			return errors.New("apps schema requires NAME")
		}
		return clientCallOutput(ctx, stdout, client, "schema", control.NameArgs{Name: args[1]})
	default:
		_ = result
		return errors.New("unknown apps subcommand")
	}
}

type stringsFlag []string

func (values *stringsFlag) String() string         { return strings.Join(*values, ",") }
func (values *stringsFlag) Set(value string) error { *values = append(*values, value); return nil }

func queryCommand(stdout io.Writer, home string, args []string) error {
	flags := flag.NewFlagSet("query", flag.ContinueOnError)
	sqlText := flags.String("sql", "", "read-only SQL")
	expectedRevision := flags.String("expected-revision", "", "expected active revision")
	expectedPosition := flags.Int64("expected-position", -1, "expected frontier")
	rowLimit := flags.Int("limit", 0, "row limit")
	var rawParams, mountValues stringsFlag
	flags.Var(&rawParams, "param", "JSON parameter")
	flags.Var(&mountValues, "mount", "ALIAS=TAILAPP")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *sqlText == "" {
		return errors.New("query requires APP --sql SQL")
	}
	parameters := make([]any, 0, len(rawParams))
	for _, raw := range rawParams {
		var value any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if number, ok := value.(json.Number); ok {
			if integer, err := number.Int64(); err == nil {
				value = integer
			} else if real, err := number.Float64(); err == nil {
				value = real
			}
		}
		parameters = append(parameters, value)
	}
	mounts := map[string]string{}
	for _, value := range mountValues {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			return errors.New("mount must be ALIAS=TAILAPP")
		}
		mounts[parts[0]] = parts[1]
	}
	var position *int64
	if *expectedPosition >= 0 {
		position = expectedPosition
	}
	return clientCallOutput(context.Background(), stdout, control.NewClient(filepath.Join(home, "engine.sock")), "query", control.QueryArgs{Name: flags.Arg(0), SQL: *sqlText, Parameters: parameters, Mounts: mounts, ExpectedRevision: *expectedRevision, ExpectedPosition: position, RowLimit: *rowLimit})
}

func ingestFixture(ctx context.Context, stdout io.Writer, home string, args []string) error {
	flags := flag.NewFlagSet("ingest-fixture", flag.ContinueOnError)
	signalName := flags.String("signal", "logs", "logs, traces, or metrics")
	contentType := flags.String("content-type", "application/json", "OTLP content type")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("ingest-fixture requires FILE")
	}
	body, err := os.ReadFile(flags.Arg(0))
	if err != nil {
		return err
	}
	metadata, err := os.ReadFile(filepath.Join(home, "engine.json"))
	if err != nil {
		return err
	}
	var engineInfo struct {
		OTLP string `json:"otlp_http"`
	}
	if err := json.Unmarshal(metadata, &engineInfo); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, engineInfo.OTLP+"/v1/"+*signalName, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", *contentType)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	reply, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("OTLP %s: %s", response.Status, string(reply))
	}
	return output(stdout, map[string]any{"status": response.StatusCode, "records": response.Header.Get("X-Tailapp-Records"), "first": response.Header.Get("X-Tailapp-Position-First"), "last": response.Header.Get("X-Tailapp-Position-Last")})
}

func call(stdout io.Writer, home, operation string, args any) error {
	return clientCallOutput(context.Background(), stdout, control.NewClient(filepath.Join(home, "engine.sock")), operation, args)
}
func clientCallOutput(ctx context.Context, stdout io.Writer, client *control.Client, operation string, args any) error {
	var result any
	if err := client.Call(ctx, operation, args, &result); err != nil {
		return err
	}
	return output(stdout, result)
}
func output(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
