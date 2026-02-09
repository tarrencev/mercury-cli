package cligen

import (
	"fmt"
	"net/url"
	"strings"
)

func extractPathParams(path string) []string {
	var out []string
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			continue
		}
		j := strings.IndexByte(path[i:], '}')
		if j <= 1 {
			continue
		}
		name := path[i+1 : i+j]
		out = append(out, name)
		i = i + j
	}
	return out
}

func applyEnvToServerURL(serverURL string, env string) (string, error) {
	if env != "sandbox" {
		return serverURL, nil
	}
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Host) {
	case "api.mercury.com":
		u.Host = "api-sandbox.mercury.com"
	case "oauth2.mercury.com":
		u.Host = "oauth2-sandbox.mercury.com"
	}
	return u.String(), nil
}

func joinBaseAndPath(baseURL string, path string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("empty base url")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String(), nil
}
