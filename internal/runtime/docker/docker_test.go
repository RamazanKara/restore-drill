package docker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/engine"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func TestCreateLifecycle(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, tt := range []struct {
		name       string
		failPath   string
		pull       bool
		cancel     bool
		wantErr    string
		wantRemove int32
	}{
		{name: "local image"},
		{name: "pull missing image", pull: true},
		{name: "inspect image fails", failPath: "/images/redis:7-alpine/json", wantErr: "inspect image"},
		{name: "pull fails", pull: true, failPath: "/images/create", wantErr: "pull image"},
		{name: "create fails", failPath: "/containers/create", wantErr: "create container"},
		{name: "start fails", failPath: "/containers/" + id + "/start", wantErr: "start container", wantRemove: 1},
		{name: "inspect container fails", failPath: "/containers/" + id + "/json", wantErr: "inspect container", wantRemove: 1},
		{name: "canceled start", cancel: true, wantErr: "start container", wantRemove: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var removed, pulled atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/v1.47")
				w.Header().Set("Content-Type", "application/json")
				if path == tt.failPath {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = io.WriteString(w, `{"message":"daemon failed"}`)
					return
				}
				switch path {
				case "/images/redis:7-alpine/json":
					if tt.pull {
						w.WriteHeader(http.StatusNotFound)
					}
					_, _ = io.WriteString(w, `{}`)
				case "/images/create":
					pulled.Add(1)
					_, _ = io.WriteString(w, `{"status":"Download complete"}`)
				case "/containers/create":
					var spec struct {
						dockercontainer.Config
						HostConfig dockercontainer.HostConfig
					}
					if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
						t.Errorf("decode create request: %v", err)
					}
					if spec.Image != "redis:7-alpine" || len(spec.Env) != 1 || spec.Env[0] != "KEY=value" {
						t.Errorf("unexpected target config: %+v", spec.Config)
					}
					bindings := spec.HostConfig.PortBindings["6379/tcp"]
					if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" || bindings[0].HostPort != "0" {
						t.Errorf("ports must bind only to loopback: %+v", bindings)
					}
					if spec.HostConfig.Memory != 512*1024*1024 || spec.HostConfig.NanoCPUs != 500_000_000 {
						t.Errorf("resource limits not applied: %+v", spec.HostConfig.Resources)
					}
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, `{"Id":"`+id+`"}`)
				case "/containers/" + id + "/start":
					if tt.cancel {
						cancel()
						<-r.Context().Done()
						return
					}
					w.WriteHeader(http.StatusNoContent)
				case "/containers/" + id + "/json":
					_, _ = io.WriteString(w, `{"NetworkSettings":{"Ports":{"6379/tcp":[{"HostPort":"16379"}]}}}`)
				case "/containers/" + id:
					if r.Method != http.MethodDelete || r.URL.Query().Get("force") != "1" {
						t.Errorf("expected forced removal, got %s %s", r.Method, r.URL)
					}
					removed.Add(1)
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			rt := testRuntime(t, server)
			container, err := rt.Create(ctx, engine.ContainerSpec{
				Image: "redis:7-alpine", Env: map[string]string{"KEY": "value"},
				Ports: []int{6379}, MemoryLimit: 512 * 1024 * 1024, CPULimit: 500_000_000,
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected %q error, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("create: %v", err)
				}
				if container.ID() != id[:12] || container.Host() != "127.0.0.1" || container.Port(6379) != 16379 {
					t.Fatalf("unexpected target details: %+v", container)
				}
				if tt.pull && pulled.Load() != 1 {
					t.Fatal("missing image was not pulled")
				}
			}
			if got := removed.Load(); got != tt.wantRemove {
				t.Fatalf("removed %d containers, want %d", got, tt.wantRemove)
			}
		})
	}
}

func TestDestroyRemovesContainerAfterStopFailure(t *testing.T) {
	var removed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/stop"):
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodDelete:
			removed.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	if err := testRuntime(t, server).Destroy(context.Background(), &dockerContainer{id: "target"}); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !removed.Load() {
		t.Fatal("container was not removed after stop failed")
	}
}

func testRuntime(t *testing.T, server *httptest.Server) *Runtime {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.47"), client.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return &Runtime{client: cli}
}

func TestCopyAndCloseWriteCopiesAndClosesWriteSide(t *testing.T) {
	writer := &recordingCloseWriter{}

	if err := copyAndCloseWrite(writer, strings.NewReader("backup")); err != nil {
		t.Fatalf("copy and close write: %v", err)
	}
	if writer.body != "backup" {
		t.Fatalf("expected copied body, got %q", writer.body)
	}
	if !writer.closed {
		t.Fatal("expected write side to be closed")
	}
}

func TestCopyAndCloseWriteReturnsCopyError(t *testing.T) {
	writer := &recordingCloseWriter{}

	err := copyAndCloseWrite(writer, errReader{})
	if err == nil {
		t.Fatal("expected copy error")
	}
	if !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !writer.closed {
		t.Fatal("expected write side to be closed after copy error")
	}
}

type recordingCloseWriter struct {
	body   string
	closed bool
}

func (w *recordingCloseWriter) Write(p []byte) (int, error) {
	w.body += string(p)
	return len(p), nil
}

func (w *recordingCloseWriter) CloseWrite() error {
	w.closed = true
	return nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

var _ io.Reader = errReader{}
