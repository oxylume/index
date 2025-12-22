package handler

import (
	"encoding/json"
	"net/http"

	"github.com/oxylume/index/internal/db"
	"github.com/oxylume/index/pkg/proxy"
	"github.com/xssnick/tonutils-go/ton/dns"
)

var specialNamespaces = []string{".adnl.", ".bag."}

type Handler struct {
	dns        *dns.Client
	bags       *proxy.BagProvider
	rldp       *proxy.RLDPConnector
	sites      *db.SitesStore
	zones      map[string]struct{}
	namespaces []string
}

func NewHandler(dns *dns.Client, bags *proxy.BagProvider, rldp *proxy.RLDPConnector, sites *db.SitesStore, zones []string) *Handler {
	zonesMap := make(map[string]struct{}, len(zones))
	namespaces := make([]string, 0, len(zones)+len(specialNamespaces))
	for _, zone := range zones {
		zonesMap[zone] = struct{}{}
		namespaces = append(namespaces, zone+".")
	}
	namespaces = append(namespaces, specialNamespaces...)
	return &Handler{
		dns:        dns,
		bags:       bags,
		rldp:       rldp,
		sites:      sites,
		zones:      zonesMap,
		namespaces: namespaces,
	}
}

func (h *Handler) ApiHandler(mux *http.ServeMux) http.Handler {
	mux.HandleFunc("GET /sites/stats", h.GetStats)
	mux.HandleFunc("GET /sites/random", h.GetRandomSite)
	mux.HandleFunc("GET /sites", h.GetSites)
	return corsMiddleware(mux)
}

func (h *Handler) GatewayHandler() http.Handler {
	return corsMiddleware(http.HandlerFunc(h.ServeGateway))
}

func writeJson(w http.ResponseWriter, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
