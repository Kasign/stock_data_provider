package bootstrap

import "testing"

func TestBoundedIntValue(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		fallback int
		minimum  int
		maximum  int
		want     int
		wantErr  bool
	}{
		{name: "default", raw: "", fallback: 2, minimum: 1, maximum: 32, want: 2},
		{name: "valid", raw: "8", fallback: 2, minimum: 1, maximum: 32, want: 8},
		{name: "not integer", raw: "many", fallback: 2, minimum: 1, maximum: 32, wantErr: true},
		{name: "below minimum", raw: "0", fallback: 2, minimum: 1, maximum: 32, wantErr: true},
		{name: "above maximum", raw: "33", fallback: 2, minimum: 1, maximum: 32, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := boundedIntValue(tt.raw, tt.name, tt.fallback, tt.minimum, tt.maximum)
			if (err != nil) != tt.wantErr {
				t.Fatalf("boundedIntValue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("boundedIntValue() = %d, want %d", got, tt.want)
			}
		})
	}
}
