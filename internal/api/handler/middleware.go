package handler

import (
	"net/http"
	"strings"
)

var specialNamespaces = []string{".adnl.", ".bag."}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) gatewayMiddleware(next http.Handler) http.Handler {
	namespaces := make([]string, 0, len(h.zones)+len(specialNamespaces))
	for zone := range h.zones {
		namespaces = append(namespaces, zone+".")
	}
	namespaces = append(namespaces, specialNamespaces...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, namespace := range namespaces {
			i := strings.Index(r.Host, namespace)
			if i < 0 {
				continue
			}
			r.Host = r.Host[:i+len(namespace)-1]
			h.ServeGateway(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
