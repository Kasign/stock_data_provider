// Package config provides small dependency-free configuration helpers.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnvIfExists loads KEY=VALUE entries from path without replacing any
// environment variable already set by the process (e.g. Docker or systemd).
// A missing file is not an error. It intentionally supports the common .env
// subset: blank lines, comments, optional "export", and single/double quotes.
func LoadDotEnvIfExists(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open env file: %w", err)
	}
	defer file.Close()

	type entry struct {
		key   string
		value string
	}
	entries := make([]entry, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validKey(key) {
			return fmt.Errorf("invalid .env entry at line %d", lineNo)
		}
		value = strings.TrimSpace(value)
		value = stripInlineComment(value)
		value, err = unquote(value)
		if err != nil || strings.ContainsRune(value, 0) {
			return fmt.Errorf("invalid .env value at line %d", lineNo)
		}
		entries = append(entries, entry{key: key, value: value})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file: %w", err)
	}

	// Parse and validate the complete file before mutating process environment.
	// The first occurrence wins, matching the previous load-if-absent behavior.
	seen := make(map[string]struct{}, len(entries))
	for _, item := range entries {
		if _, exists := seen[item.key]; exists {
			continue
		}
		seen[item.key] = struct{}{}
		if _, exists := os.LookupEnv(item.key); exists {
			continue
		}
		if err := os.Setenv(item.key, item.value); err != nil {
			return fmt.Errorf("set environment variable from .env: %w", err)
		}
	}
	return nil
}

func stripInlineComment(s string) string {
	if s == "" {
		return s
	}
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		if quote != 0 {
			if s[i] == quote && (i == 0 || s[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if s[i] == '\'' || s[i] == '"' {
			quote = s[i]
			continue
		}
		if s[i] == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

func unquote(s string) (string, error) {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s[1:len(s)-1], `\n`, "\n"), `\t`, "\t"), `\"`, "\""), nil
		}
		if s[0] == '\'' && s[len(s)-1] == '\'' {
			return s[1 : len(s)-1], nil
		}
	}
	if strings.HasPrefix(s, `"`) || strings.HasPrefix(s, `'`) {
		return "", errors.New("unclosed quote")
	}
	return s, nil
}
