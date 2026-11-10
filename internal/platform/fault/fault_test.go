package fault

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFaultJSONDoesNotExposeInternalFields(t *testing.T) {
	f := New(ClassUnavailable, "data.provider.unavailable", "provider unavailable", errors.New("secret token in upstream error"))
	f.Dependency = "provider.internal-host"
	f.Retryable = true

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("internal fault fields must not be serialized: %s", data)
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func TestFaultValidateEnforcesStableMetadata(t *testing.T) {
	tests := []struct {
		name    string
		fault   *Fault
		wantErr bool
	}{
		{
			name: "valid retryable dependency fault",
			fault: &Fault{
				Class: ClassUnavailable, Code: "news.provider.unavailable",
				SafeMessage: "provider unavailable", Dependency: "news.provider.tavily",
				Retryable: true,
			},
		},
		{
			name: "retryable validation fault",
			fault: &Fault{
				Class: ClassValidation, Code: "data.input.invalid",
				SafeMessage: "invalid input", Retryable: true,
			},
			wantErr: true,
		},
		{
			name: "URL dependency",
			fault: &Fault{
				Class: ClassUnavailable, Code: "storage.unavailable",
				SafeMessage: "storage unavailable", Dependency: "https://db.example/path",
			},
			wantErr: true,
		},
		{
			name: "IP dependency",
			fault: &Fault{
				Class: ClassUnavailable, Code: "storage.unavailable",
				SafeMessage: "storage unavailable", Dependency: "127.0.0.1",
			},
			wantErr: true,
		},
		{
			name: "IPv6 dependency",
			fault: &Fault{
				Class: ClassUnavailable, Code: "storage.unavailable",
				SafeMessage: "storage unavailable", Dependency: "2001:4860:4860::8888",
			},
			wantErr: true,
		},
		{
			name: "IPv4-mapped IPv6 dependency",
			fault: &Fault{
				Class: ClassUnavailable, Code: "storage.unavailable",
				SafeMessage: "storage unavailable", Dependency: "::ffff:8.8.8.8",
			},
			wantErr: true,
		},
		{
			name: "unstable code",
			fault: &Fault{
				Class: ClassInternal, Code: "Internal Error",
				SafeMessage: "operation failed",
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.fault.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
