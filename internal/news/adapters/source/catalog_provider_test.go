package source

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
)

func TestStaticSourceCatalogRequiresCompleteProviderBinding(t *testing.T) {
	registration := app.SourceEndpointRegistration{
		SourceID:                      "cninfo",
		EndpointID:                    "cninfo-his-announcement",
		SourcePolicyVersionID:         "cninfo-source-policy-v1",
		ProviderCode:                  "cninfo",
		ProviderOutputPolicyRef:       "cninfo-provider-output-policy",
		ProviderOutputPolicyVersionID: "cninfo-provider-output-policy-v1",
	}
	catalog, err := NewStaticSourceCatalog([]app.SourceEndpointRegistration{registration})
	if err != nil {
		t.Fatal(err)
	}
	got, err := catalog.LookupEndpoint(context.Background(), registration.SourceID, registration.EndpointID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderCode != registration.ProviderCode ||
		got.ProviderOutputPolicyRef != registration.ProviderOutputPolicyRef ||
		got.ProviderOutputPolicyVersionID != registration.ProviderOutputPolicyVersionID {
		t.Fatalf("provider binding not preserved: %+v", got)
	}
}

func TestStaticSourceCatalogRejectsPartialProviderBinding(t *testing.T) {
	registration := app.SourceEndpointRegistration{
		SourceID:                      "cninfo",
		EndpointID:                    "cninfo-his-announcement",
		SourcePolicyVersionID:         "cninfo-source-policy-v1",
		ProviderCode:                  "cninfo",
		ProviderOutputPolicyVersionID: "cninfo-provider-output-policy-v1",
	}
	if _, err := NewStaticSourceCatalog([]app.SourceEndpointRegistration{registration}); !errors.Is(err, ErrInvalidSourceCatalog) {
		t.Fatalf("partial provider binding error=%v, want ErrInvalidSourceCatalog", err)
	}
}
