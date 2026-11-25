package storage

import (
	"context"
	"testing"
	"time"
)

func TestResearchMariaDBPhysicalSchemaMatchesMigrationBaseline(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ValidateSchema(ctx, db); err != nil {
		t.Fatalf("Research physical schema does not match its migration baseline: %v", err)
	}
}
