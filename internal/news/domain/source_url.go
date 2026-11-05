package domain

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

// SourceURLPolicy 是新闻领域对来源目标 URL 的允许范围。
// AllowedHosts 使用精确主机名匹配；子域名必须逐个登记，不支持隐式通配符。
type SourceURLPolicy struct {
	AllowedSchemes []string
	AllowedHosts   []string
	AllowedPorts   []uint16
}

// SourceURLDecision 返回 URL 规范化结果和稳定的业务拒绝原因。
// DNS/IP 特殊地址、DNS rebinding 和实际拨号校验仍必须由公共平台 HTTP Client 执行。
type SourceURLDecision struct {
	Allowed      bool
	CanonicalURL string
	Host         string
	Port         uint16
	ReasonCode   string
}

// EvaluateSourceURL 校验来源 URL 的协议、精确主机和端口范围，并规范化主机、默认端口及 Fragment。
func EvaluateSourceURL(rawURL string, policy SourceURLPolicy) SourceURLDecision {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" {
		return sourceURLDenied("news.source_url.invalid")
	}
	if parsed.User != nil {
		return sourceURLDenied("news.source_url.userinfo_denied")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !containsPolicyValue(normalizeSchemes(policy.AllowedSchemes), scheme) {
		return sourceURLDenied("news.source_url.scheme_denied")
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" || strings.Contains(host, "%") || !validSourceHost(host) {
		return sourceURLDenied("news.source_url.host_invalid")
	}
	if !containsPolicyValue(normalizeHosts(policy.AllowedHosts), host) {
		return sourceURLDenied("news.source_url.host_not_allowed")
	}

	portText := parsed.Port()
	if strings.HasSuffix(parsed.Host, ":") {
		return sourceURLDenied("news.source_url.port_invalid")
	}
	defaultPort := uint16(443)
	if scheme == "http" {
		defaultPort = 80
	}
	port := defaultPort
	explicitPort := portText != ""
	if explicitPort {
		value, parseErr := strconv.Atoi(portText)
		if parseErr != nil || value < 1 || value > 65535 {
			return sourceURLDenied("news.source_url.port_invalid")
		}
		port = uint16(value)
		if port != defaultPort && !containsPolicyValue(policy.AllowedPorts, port) {
			return sourceURLDenied("news.source_url.port_not_allowed")
		}
	}

	parsed.Scheme = scheme
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	if explicitPort && port != defaultPort {
		parsed.Host = net.JoinHostPort(host, strconv.Itoa(int(port)))
	} else if strings.Contains(host, ":") {
		parsed.Host = "[" + host + "]"
	} else {
		parsed.Host = host
	}

	return SourceURLDecision{
		Allowed:      true,
		CanonicalURL: parsed.String(),
		Host:         host,
		Port:         port,
	}
}

func sourceURLDenied(reason string) SourceURLDecision {
	return SourceURLDecision{Allowed: false, ReasonCode: reason}
}

func normalizeSchemes(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strings.ToLower(strings.TrimSpace(value)))
	}
	return result
}

func normalizeHosts(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if host != "" {
			if ip := net.ParseIP(host); ip != nil {
				host = ip.String()
			}
			result = append(result, host)
		}
	}
	return result
}

func validSourceHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}
