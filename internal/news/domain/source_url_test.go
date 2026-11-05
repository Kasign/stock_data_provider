package domain

import "testing"

func sourceURLPolicy() SourceURLPolicy {
	return SourceURLPolicy{
		AllowedSchemes: []string{"https"},
		AllowedHosts:   []string{"news.example.com", "2001:db8::1"},
		AllowedPorts:   []uint16{8443},
	}
}

func TestEvaluateSourceURLCanonicalizesHostDefaultPortAndFragment(t *testing.T) {
	decision := EvaluateSourceURL("HTTPS://News.Example.Com.:443/a%2Fb?b=2&a=1#section", sourceURLPolicy())
	if !decision.Allowed {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.CanonicalURL != "https://news.example.com/a%2Fb?b=2&a=1" {
		t.Fatalf("canonical URL=%q", decision.CanonicalURL)
	}
	if decision.Host != "news.example.com" || decision.Port != 443 {
		t.Fatalf("target=%+v", decision)
	}
}

func TestEvaluateSourceURLRequiresExactHostAndExplicitNonDefaultPort(t *testing.T) {
	decision := EvaluateSourceURL("https://sub.news.example.com/article", sourceURLPolicy())
	if decision.Allowed || decision.ReasonCode != "news.source_url.host_not_allowed" {
		t.Fatalf("host decision=%+v", decision)
	}

	decision = EvaluateSourceURL("https://news.example.com:9443/article", sourceURLPolicy())
	if decision.Allowed || decision.ReasonCode != "news.source_url.port_not_allowed" {
		t.Fatalf("port decision=%+v", decision)
	}

	decision = EvaluateSourceURL("https://news.example.com:8443/article", sourceURLPolicy())
	if !decision.Allowed || decision.Port != 8443 {
		t.Fatalf("explicit port decision=%+v", decision)
	}
}

func TestEvaluateSourceURLRejectsUnsafeURLForms(t *testing.T) {
	cases := []struct {
		name string
		url  string
		code string
	}{
		{name: "userinfo", url: "https://user:pass@news.example.com/a", code: "news.source_url.userinfo_denied"},
		{name: "scheme", url: "file:///etc/passwd", code: "news.source_url.invalid"},
		{name: "scheme not allowed", url: "http://news.example.com/a", code: "news.source_url.scheme_denied"},
		{name: "invalid port", url: "https://news.example.com:0/a", code: "news.source_url.port_invalid"},
		{name: "host suffix attack", url: "https://news.example.com.evil.test/a", code: "news.source_url.host_not_allowed"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decision := EvaluateSourceURL(testCase.url, sourceURLPolicy())
			if decision.Allowed || decision.ReasonCode != testCase.code {
				t.Fatalf("decision=%+v", decision)
			}
		})
	}
}

func TestEvaluateSourceURLNormalizesIPv6Literal(t *testing.T) {
	decision := EvaluateSourceURL("https://[2001:0db8:0:0:0:0:0:1]:443/path", sourceURLPolicy())
	if !decision.Allowed {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.CanonicalURL != "https://[2001:db8::1]/path" || decision.Host != "2001:db8::1" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestEvaluateSourceURLRejectsEmptyAllowlist(t *testing.T) {
	decision := EvaluateSourceURL("https://news.example.com/a", SourceURLPolicy{
		AllowedSchemes: []string{"https"},
	})
	if decision.Allowed || decision.ReasonCode != "news.source_url.host_not_allowed" {
		t.Fatalf("decision=%+v", decision)
	}
}
