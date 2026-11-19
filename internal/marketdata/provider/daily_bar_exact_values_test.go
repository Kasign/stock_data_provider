package provider

import "testing"

func TestCanonicalDailyBarDecimal(t *testing.T) {
	cases := map[string]string{
		"001.2300": "1.23",
		"0": "0",
		"0.000": "0",
		"1.23e-3": "0.00123",
		".5": "0.5",
		"1.": "1",
		"-0.000": "0",
		"1000e-3": "1",
	}
	for input, want := range cases {
		got, err := canonicalDailyBarDecimal(input)
		if err != nil || got != want {
			t.Errorf("canonicalDailyBarDecimal(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", ".", "NaN", "1.2.3", "1e101", "--1"} {
		if _, err := canonicalDailyBarDecimal(input); err == nil {
			t.Errorf("canonicalDailyBarDecimal(%q) must reject invalid input", input)
		}
	}
}

func TestDailyBarExactValuesFromJSONPreservesNumericTokens(t *testing.T) {
	got := dailyBarExactValuesFromJSON(
		[]byte(`"001.2300"`), []byte("1.25"), []byte("1.20"), []byte("1.24"),
		[]byte("123.00"), []byte("456.7800"), "CNY", "CNY",
	)
	if got == nil || got.Open != "1.23" || got.High != "1.25" || got.RawVolume != "123" ||
		got.RawAmount == nil || *got.RawAmount != "456.78" || got.AmountCNY == nil || *got.AmountCNY != "456.78" {
		t.Fatalf("unexpected exact JSON values: %+v", got)
	}
}
