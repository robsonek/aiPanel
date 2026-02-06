package hosting

import (
	"fmt"
	"regexp"
	"strings"
)

var domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

func normalizeDomain(domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return "", fmt.Errorf("domain is required")
	}
	if !domainPattern.MatchString(domain) {
		return "", fmt.Errorf("invalid domain")
	}
	return domain, nil
}

func sanitizeToken(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if r == '-' || r == '_' || r == '.' {
			b.WriteRune('-')
		}
	}
	token := strings.Trim(b.String(), "-")
	if token == "" {
		return "site"
	}
	return token
}

func poolName(domain, phpVersion string) string {
	ver := strings.ReplaceAll(strings.TrimSpace(phpVersion), ".", "")
	if ver == "" {
		ver = "83"
	}
	base := sanitizeToken(domain)
	name := fmt.Sprintf("%s-php%s", base, ver)
	if len(name) > 48 {
		return name[:48]
	}
	return name
}

func socketPath(domain, phpVersion string) string {
	return "/run/php/" + poolName(domain, phpVersion) + ".sock"
}
