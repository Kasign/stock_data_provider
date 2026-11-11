package mysqlredis

import (
	"context"
	"testing"
)

func TestNewWithResourcesRequiresConnections(t *testing.T) {
	if _, err := NewWithResources(context.Background(), Config{}, nil, nil); err == nil {
		t.Fatal("NewWithResources() error = nil, want missing resource error")
	}
}

func TestSQLConnectorRejectsInvalidConfig(t *testing.T) {
	if _, err := (Config{}).SQLConnector(); err == nil {
		t.Fatal("SQLConnector() error = nil, want invalid config error")
	}
}
