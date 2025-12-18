package handler

import (
	"encoding/json"
	"net/http"

	"github.com/oxylume/index/internal/db"
	"github.com/oxylume/index/pkg/proxy"
	"github.com/xssnick/tonutils-go/ton/dns"
)

type Handler struct {
	dns   *dns.Client
	bags  *proxy.BagProvider
	rldp  *proxy.RLDPConnector
	sites *db.SitesStore
	zones map[string]struct{}
}

func NewHandler(dns *dns.Client, bags *proxy.BagProvider, rldp *proxy.RLDPConnector, sites *db.SitesStore, zones []string) *Handler {
	zonesMap := make(map[string]struct{}, len(zones))
	for _, zone := range zones {
		zonesMap[zone] = struct{}{}
	}
	return &Handler{
		dns:   dns,
		bags:  bags,
		rldp:  rldp,
		sites: sites,
		zones: zonesMap,
	}
}

func (h *Handler) HttpHandler(mux *http.ServeMux, enableGateway bool) http.Handler {
	mux.HandleFunc("GET /sites/stats", h.GetStats)
	mux.HandleFunc("GET /sites/random", h.GetRandomSite)
	mux.HandleFunc("GET /sites", h.GetSites)
	handler := mux
	if enableGateway {
		h.gatewayMiddleware(handler)
	}
	return corsMiddleware(handler)
}

func writeJson(w http.ResponseWriter, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
