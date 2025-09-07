package handler

import (
	"fmt"
	"net/http"

	"github.com/oxylume/index/internal/api"
	"github.com/oxylume/index/internal/db"
)

const (
	maxLimit = 100
)

type getStatsResponse struct {
	Domains int `json:"domains"`
	Sites   int `json:"sites"`
	Active  int `json:"active"`
}

type getSitesResponse struct {
	Sites  []siteResponse `json:"sites"`
	Cursor string         `json:"cursor,omitempty"`
}

type siteResponse struct {
	Domain       string `json:"domain"`
	Unicode      string `json:"unicode"`
	Accessible   bool   `json:"accessible"`
	InStorage    bool   `json:"inStorage"`
	SpamContent  bool   `json:"spamContent"`
	CheckedUtime int64  `json:"checkedUtime"`
}

var allowedZones = map[string]struct{}{
	".ton":  {},
	".t.me": {},
}

var allowedSortBy = map[db.SortBy]struct{}{
	db.SortByDomain:    {},
	db.SortByCheckedAt: {},
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.sites.GetStats(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("internal error: %v", err), http.StatusInternalServerError)
		return
	}
	resp := getStatsResponse{
		Domains: stats.TotalDomains,
		Sites:   stats.TotalSites,
		Active:  stats.ActiveSites,
	}
	writeJson(w, resp)
}

func (h *Handler) GetRandomSite(w http.ResponseWriter, r *http.Request) {
	site, err := h.sites.GetRandomSite(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("internal error: %v", err), http.StatusInternalServerError)
		return
	}
	writeJson(w, siteToResponse(*site))
}

func (h *Handler) GetSites(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var params db.ListFilters
	params.Search = query.Get("search")
	if v, ok := api.GetBool(query, "inaccessible"); ok {
		params.Inaccessible = v
	}
	if v, ok := api.GetBool(query, "punycode"); ok {
		params.Punycode = &v
	}
	if v, ok := api.GetBool(query, "spam"); ok {
		params.Spam = v
	}
	if v := query.Get("zone"); v != "" {
		if _, ok := allowedZones[v]; !ok {
			http.Error(w, fmt.Sprintf("invalid zone %s", v), http.StatusBadRequest)
			return
		}
		params.Zone = v
	}
	params.SortBy = db.SortByDomain
	if v := query.Get("sort"); v != "" {
		if _, ok := allowedSortBy[db.SortBy(v)]; !ok {
			http.Error(w, fmt.Sprintf("invalid sort value %s", v), http.StatusBadRequest)
			return
		}
		params.SortBy = db.SortBy(v)
	}
	if v, ok := api.GetBool(query, "desc"); ok {
		params.Desc = v
	}
	var cursor *db.Cursor
	if v := query.Get("cursor"); v != "" {
		parsed, err := api.DecodeCursor(v, params.SortBy)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to parse cursor: %v", err), http.StatusBadRequest)
			return
		}
		cursor = parsed
	}
	limit := 50
	if v, ok, err := api.GetInt(query, "limit"); err != nil {
		http.Error(w, fmt.Sprintf("unable to parse limit: %v", err), http.StatusBadRequest)
		return
	} else if ok {
		if v < 0 || v > maxLimit {
			http.Error(w, fmt.Sprintf("limit must be between 0 and %d", maxLimit), http.StatusBadRequest)
			return
		}
		limit = v
	}

	sites, nextCursor, err := h.sites.List(r.Context(), &params, cursor, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("internal error: %v", err), http.StatusInternalServerError)
		return
	}

	respSites := make([]siteResponse, len(sites))
	for i, item := range sites {
		respSites[i] = siteToResponse(item)
	}
	var respCursor string
	if nextCursor != nil {
		respCursor = api.EncodeCursor(nextCursor)
	}
	resp := getSitesResponse{
		Sites:  respSites,
		Cursor: respCursor,
	}
	writeJson(w, resp)
}

func siteToResponse(site db.Site) siteResponse {
	return siteResponse{
		Domain:       site.Domain,
		Unicode:      site.Unicode,
		Accessible:   site.Status == db.StatusAccessible,
		InStorage:    site.InStorage,
		SpamContent:  site.SpamContent,
		CheckedUtime: site.CheckedAt.Unix(),
	}
}
