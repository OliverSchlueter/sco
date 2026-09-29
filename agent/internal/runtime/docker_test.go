package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/go-sdk/client"
	mclient "github.com/moby/moby/client"
)

func TestDockerClientWithoutCLIConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		host string
	}{
		{name: "default socket", host: mclient.DefaultDockerHost},
		{name: "explicit host", host: "tcp://127.0.0.1:2375"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("DOCKER_CONFIG", "")
			t.Setenv("DOCKER_AUTH_CONFIG", "")
			t.Setenv("DOCKER_CONTEXT", "")
			t.Setenv("DOCKER_HOST", "")
			t.Setenv("DOCKER_TLS_VERIFY", "")
			t.Setenv("XDG_RUNTIME_DIR", "")
			if tc.name == "explicit host" {
				t.Setenv("DOCKER_HOST", tc.host)
			}
			opts, err := dockerClientOptions()
			if err != nil {
				t.Fatal(err)
			}
			// Exercise SDK initialization without requiring a Docker daemon.
			opts = append(opts, client.WithHealthCheck(func(context.Context) func(client.SDKClient) error {
				return func(client.SDKClient) error { return nil }
			}))
			cli, err := client.New(context.Background(), opts...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cli.Close() })
			if got := cli.DaemonHost(); got != tc.host {
				t.Fatalf("Docker host = %q, want %q", got, tc.host)
			}
			if _, err := os.Stat(filepath.Join(home, ".docker")); !os.IsNotExist(err) {
				t.Fatalf("initialization unexpectedly created Docker configuration: %v", err)
			}
		})
	}
}

func TestDockerClientPreservesConfiguration(t *testing.T) {
	for _, name := range []string{"existing file", "invalid file", "context", "auth config", "missing override directory"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DOCKER_CONFIG", dir)
			t.Setenv("DOCKER_AUTH_CONFIG", "")
			t.Setenv("DOCKER_CONTEXT", "")
			t.Setenv("DOCKER_HOST", "")
			t.Setenv("DOCKER_TLS_VERIFY", "")
			t.Setenv("XDG_RUNTIME_DIR", "")
			switch name {
			case "existing file", "invalid file":
				data := "{}"
				if name == "invalid file" {
					data = "invalid JSON"
				}
				if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			case "context":
				t.Setenv("DOCKER_CONTEXT", "remote")
			case "auth config":
				t.Setenv("DOCKER_AUTH_CONFIG", "{}")
			case "missing override directory":
				t.Setenv("DOCKER_CONFIG", filepath.Join(dir, "missing"))
			}
			opts, err := dockerClientOptions()
			if err != nil {
				t.Fatal(err)
			}
			opts = append(opts, client.WithHealthCheck(func(context.Context) func(client.SDKClient) error {
				return func(client.SDKClient) error { return nil }
			}))
			cli, err := client.New(context.Background(), opts...)
			wantError := name == "invalid file" || name == "context"
			if (err != nil) != wantError {
				t.Fatalf("Docker initialization error = %v, want error: %v", err, wantError)
			}
			if cli != nil {
				t.Cleanup(func() { _ = cli.Close() })
			}
		})
	}
}
