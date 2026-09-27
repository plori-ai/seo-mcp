package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestClientFromEnv(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("sample:password"))
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"api key", map[string]string{"DATAFORSEO_API_KEY": key}, key},
		{"trim api key", map[string]string{"DATAFORSEO_API_KEY": " \n" + key + "\n"}, key},
		{"login and password", map[string]string{"DATAFORSEO_LOGIN": "sample", "DATAFORSEO_PASSWORD": "password"}, key},
		{"key takes precedence", map[string]string{"DATAFORSEO_API_KEY": key, "DATAFORSEO_LOGIN": "other", "DATAFORSEO_PASSWORD": "other"}, key},
		{"missing", nil, ""},
		{"login only", map[string]string{"DATAFORSEO_LOGIN": "sample"}, ""},
		{"password only", map[string]string{"DATAFORSEO_PASSWORD": "password"}, ""},
		{"blank key", map[string]string{"DATAFORSEO_API_KEY": " \n"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := clientFromEnv(func(name string) string { return tt.env[name] })
			if tt.want == "" {
				if err == nil || client != nil {
					t.Fatalf("got (%v, %v), want missing credentials error", client, err)
				}
				for _, name := range []string{"DATAFORSEO_API_KEY", "DATAFORSEO_LOGIN", "DATAFORSEO_PASSWORD"} {
					if !strings.Contains(err.Error(), name) {
						t.Errorf("error %q does not name %s", err, name)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if client.APIKey != tt.want {
				t.Error("wrong API key")
			}
		})
	}
}

func TestRunStartup(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr string
	}{
		{"version without credentials", []string{"-version"}, version + "\n", ""},
		{"missing credentials", nil, "", "DATAFORSEO_API_KEY"},
		{"positional argument", []string{"extra"}, "", "unexpected positional arguments"},
		{"unknown flag", []string{"-unknown"}, "", "flag provided but not defined"},
		{"help", []string{"-help"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(context.Background(), tt.args, func(string) string { return "" }, &stdout, &stderr)
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			if stdout.String() != tt.wantOut {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

func TestRunRejectsUnsupportedDefaultMarket(t *testing.T) {
	getenv := func(name string) string {
		if name == "DATAFORSEO_API_KEY" {
			return "dGVzdDp0ZXN0"
		}
		return ""
	}
	for _, args := range [][]string{
		{"-location-code", "1"},
		{"-location-code", "2840", "-language-code", "xx"},
	} {
		var stdout, stderr bytes.Buffer
		err := run(context.Background(), args, getenv, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "invalid default market") {
			t.Fatalf("run(%v) error = %v, want an invalid default market error", args, err)
		}
	}
}
