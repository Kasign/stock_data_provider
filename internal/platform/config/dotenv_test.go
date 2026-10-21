package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvIfExists(t *testing.T) {
	t.Setenv("PYTHON_API_TEST_PREEXISTING", "process-value")
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nPYTHON_API_TEST_A=hello\nPYTHON_API_TEST_B=\"hello world\" # note\nexport PYTHON_API_TEST_C='quoted=value'\nPYTHON_API_TEST_PREEXISTING=from-file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotEnvIfExists(path); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"PYTHON_API_TEST_A":           "hello",
		"PYTHON_API_TEST_B":           "hello world",
		"PYTHON_API_TEST_C":           "quoted=value",
		"PYTHON_API_TEST_PREEXISTING": "process-value",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLoadMissingDotEnvIsOK(t *testing.T) {
	if err := LoadDotEnvIfExists(filepath.Join(t.TempDir(), ".missing")); err != nil {
		t.Fatal(err)
	}
}
