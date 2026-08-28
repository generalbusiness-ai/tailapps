// Package control exposes the engine's typed application operations over a
// private Unix-domain HTTP socket. CLI and MCP are clients of this one surface.
package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/generalbusiness-ai/tailapp/internal/definition"
	"github.com/generalbusiness-ai/tailapp/internal/engine"
	"github.com/generalbusiness-ai/tailapp/internal/query"
)

const MaxRequestBytes = 1 << 20

type Request struct {
	Operation string          `json:"operation"`
	Args      json.RawMessage `json:"args,omitempty"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

type CreateArgs struct {
	Name           string `json:"name"`
	Bundle         string `json:"bundle,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}
type NameArgs struct {
	Name string `json:"name"`
}
type DeleteArgs struct {
	Name           string `json:"name"`
	IdempotencyKey string `json:"idempotency_key"`
}
type PutArgs struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Content          []byte `json:"content"`
	ExpectedRevision string `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}
type RemoveArgs struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	ExpectedRevision string `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}
type ValidateArgs struct {
	Name             string `json:"name"`
	ExpectedRevision string `json:"expected_revision"`
}
type ActivateArgs struct {
	Name             string `json:"name"`
	ExpectedRevision string `json:"expected_revision"`
	Mode             string `json:"mode"`
	AcknowledgeReset bool   `json:"acknowledge_reset"`
	IdempotencyKey   string `json:"idempotency_key"`
}
type QueryArgs struct {
	Name             string            `json:"name"`
	SQL              string            `json:"sql"`
	Parameters       []any             `json:"parameters,omitempty"`
	Mounts           map[string]string `json:"mounts,omitempty"`
	ExpectedRevision string            `json:"expected_revision,omitempty"`
	ExpectedPosition *int64            `json:"expected_position,omitempty"`
	RowLimit         int               `json:"row_limit,omitempty"`
}

type Server struct {
	Engine     *engine.Engine
	mutationMu sync.Mutex
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/v1/control" {
		http.NotFound(writer, request)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxRequestBytes+1))
	if err != nil || len(body) > MaxRequestBytes {
		write(writer, nil, errors.New("control request too large"))
		return
	}
	var envelope Request
	if err := json.Unmarshal(body, &envelope); err != nil {
		write(writer, nil, err)
		return
	}
	result, err := server.dispatch(request.Context(), envelope)
	write(writer, result, err)
}

func (server *Server) dispatch(ctx context.Context, request Request) (any, error) {
	decode := func(target any) error {
		if len(request.Args) == 0 {
			return nil
		}
		decoder := json.NewDecoder(bytes.NewReader(request.Args))
		decoder.DisallowUnknownFields()
		return decoder.Decode(target)
	}
	switch request.Operation {
	case "health", "status":
		return server.Engine.Status(ctx)
	case "apps_list":
		return server.Engine.Apps(ctx)
	case "app_create":
		var args CreateArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.idempotent(ctx, request.Operation, args.IdempotencyKey, args, func() (any, error) {
			return server.Engine.Create(ctx, args.Name, args.Bundle)
		})
	case "app_get":
		var args NameArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		app, sources, err := server.Engine.App(ctx, args.Name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"app": app, "sources": sources}, nil
	case "app_delete":
		var args DeleteArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.idempotent(ctx, request.Operation, args.IdempotencyKey, args, func() (any, error) {
			return map[string]bool{"deleted": true}, server.Engine.Delete(ctx, args.Name)
		})
	case "element_put":
		var args PutArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.idempotent(ctx, request.Operation, args.IdempotencyKey, args, func() (any, error) {
			return server.Engine.Put(ctx, args.Name, args.Path, args.Content, args.ExpectedRevision)
		})
	case "element_delete":
		var args RemoveArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.idempotent(ctx, request.Operation, args.IdempotencyKey, args, func() (any, error) {
			return server.Engine.RemoveElement(ctx, args.Name, args.Path, args.ExpectedRevision)
		})
	case "validate":
		var args ValidateArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.Engine.Validate(ctx, args.Name, args.ExpectedRevision)
	case "activate":
		var args ActivateArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.idempotent(ctx, request.Operation, args.IdempotencyKey, args, func() (any, error) {
			return server.Engine.Activate(ctx, args.Name, args.ExpectedRevision, args.Mode, args.AcknowledgeReset)
		})
	case "schema":
		var args NameArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.Engine.Schema(args.Name)
	case "query":
		var args QueryArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return server.Engine.Query(ctx, args.Name, query.Request{SQL: args.SQL, Parameters: args.Parameters, ExpectedRevision: args.ExpectedRevision, ExpectedPosition: args.ExpectedPosition, RowLimit: args.RowLimit}, args.Mounts)
	default:
		return nil, errors.New("unknown control operation")
	}
}

type replayedError struct {
	code    string
	message string
}

func (err *replayedError) Error() string { return err.message }

func (server *Server) idempotent(ctx context.Context, operation, key string, args any, action func() (any, error)) (any, error) {
	if err := validateIdempotencyKey(key); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(operation))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))

	server.mutationMu.Lock()
	defer server.mutationMu.Unlock()
	record, replay, err := server.Engine.BeginMutation(ctx, key, operation, digest)
	if err != nil {
		return nil, err
	}
	if replay {
		if record.ErrorCode != "" {
			return nil, &replayedError{code: record.ErrorCode, message: record.ErrorMessage}
		}
		return json.RawMessage(record.Response), nil
	}
	result, actionErr := action()
	var response []byte
	if actionErr == nil {
		response, err = json.Marshal(result)
		if err != nil {
			actionErr = err
		}
	}
	completed := definition.MutationRecord{Response: response}
	if actionErr != nil {
		completed.ErrorCode = errorCode(actionErr)
		completed.ErrorMessage = actionErr.Error()
	}
	completeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := server.Engine.CompleteMutation(completeCtx, key, operation, digest, completed); err != nil {
		return nil, fmt.Errorf("%w: could not durably record mutation outcome: %v", definition.ErrIdempotencyInDoubt, err)
	}
	return result, actionErr
}

func validateIdempotencyKey(key string) error {
	if len(key) == 0 || len(key) > 128 || strings.TrimSpace(key) != key {
		return errors.New("idempotency_key must contain 1 to 128 non-space-bounded printable ASCII characters")
	}
	for _, character := range key {
		if character < 0x21 || character > 0x7e {
			return errors.New("idempotency_key must contain 1 to 128 non-space-bounded printable ASCII characters")
		}
	}
	return nil
}

func write(writer http.ResponseWriter, result any, err error) {
	writer.Header().Set("Content-Type", "application/json")
	response := Response{OK: err == nil}
	if err != nil {
		response.Error = &Error{Code: errorCode(err), Message: err.Error()}
	} else {
		encoded, _ := json.Marshal(result)
		response.Result = encoded
	}
	_ = json.NewEncoder(writer).Encode(response)
}
func errorCode(err error) string {
	var replayed *replayedError
	switch {
	case errors.As(err, &replayed):
		return replayed.code
	case errors.Is(err, sql.ErrNoRows):
		return "not_found"
	case errors.Is(err, definition.ErrRevisionChanged):
		return "revision_changed"
	case errors.Is(err, definition.ErrIdempotencyConflict):
		return "idempotency_conflict"
	case errors.Is(err, definition.ErrIdempotencyInDoubt):
		return "idempotency_in_doubt"
	case errors.Is(err, engine.ErrProjectionUnavailable):
		return "projection_unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case strings.Contains(err.Error(), "frontier_changed"):
		return "frontier_changed"
	case strings.Contains(err.Error(), "query_budget_exceeded"):
		return "query_budget_exceeded"
	default:
		return "operation_failed"
	}
}

func Listen(socket string) (net.Listener, error) {
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("control socket path exists and is not a socket")
		}
		if err := os.Remove(socket); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

type Client struct{ http *http.Client }

func NewClient(socket string) *Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: 5 * time.Second}
		return dialer.DialContext(ctx, "unix", socket)
	}}
	return &Client{http: &http.Client{Transport: transport, Timeout: 10 * time.Second}}
}
func (client *Client) Call(ctx context.Context, operation string, args, result any) error {
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return err
	}
	body, err := json.Marshal(Request{Operation: operation, Args: encodedArgs})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/v1/control", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var envelope Response
	if err := json.NewDecoder(io.LimitReader(response.Body, MaxRequestBytes+1)).Decode(&envelope); err != nil {
		return err
	}
	if !envelope.OK {
		if envelope.Error == nil {
			return errors.New("control operation failed")
		}
		return fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if result != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}
