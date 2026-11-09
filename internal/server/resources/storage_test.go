package resources

import (
	"testing"

	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
)

func TestOpenLegacyAnalysisStorageRequiresContext(t *testing.T) {
	if _, _, err := OpenLegacyAnalysisStorage(nil, mysqlredis.Config{}); err == nil {
		t.Fatal("OpenLegacyAnalysisStorage() error = nil, want context error")
	}
}

func TestStorageCloseIsNilSafe(t *testing.T) {
	var storage *Storage
	if err := storage.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestResourcesCloseIsIdempotent(t *testing.T) {
	resources := &Resources{}
	if err := resources.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := resources.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
