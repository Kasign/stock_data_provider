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
		if !ok || key == "" || strings.ContainsAny(key, " \t\r\n") {
			return fmt.Errorf("invalid .env entry at line %d", lineNo)
		}
		value = strings.TrimSpace(value)
		value = stripInlineComment(value)
		value, err = unquote(value)
		if err != nil {
			return fmt.Errorf("invalid .env value at line %d: %w", lineNo, err)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set environment variable from .env line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file: %w", err)
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
