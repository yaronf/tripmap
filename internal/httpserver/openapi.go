package httpserver

import (
	"net/http"
	"strings"

	"github.com/yaronf/tripmap/api"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	base := s.cfg.PublicBaseURL
	if base == "" {
		proto := r.Header.Get("X-Forwarded-Proto")
		if proto == "" {
			if r.TLS != nil {
				proto = "https"
			} else {
				// Express Mode / ALB terminate TLS; public https base for clients.
				proto = "https"
			}
		}
		host := r.Host
		if host == "" {
			host = "localhost:8080"
			proto = "http"
		}
		base = proto + "://" + host
	}
	_, _ = w.Write([]byte(OpenAPIDocument(base)))
}

// OpenAPIDocument returns the OpenAPI YAML with {{BASE_URL}} replaced.
func OpenAPIDocument(baseURL string) string {
	return strings.ReplaceAll(api.OpenAPIYAML, "{{BASE_URL}}", baseURL)
}

// OpenAPIDocumentTemplate returns the OpenAPI YAML with the {{BASE_URL}} placeholder.
func OpenAPIDocumentTemplate() string {
	return api.OpenAPIYAML
}
