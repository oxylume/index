package handler

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/oxylume/index/internal/api"
	"github.com/oxylume/index/pkg/proxy"
)

var hopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func (h *Handler) ServeGateway(w http.ResponseWriter, r *http.Request) {
	realHost, ok := h.resolveRealHost(r.Host)
	if !ok {
		http.Error(w, "provide valid TON network resource identifier as a subdomain", http.StatusBadRequest)
		return
	}
	id, inStorage, err := h.resolve(r.Context(), realHost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if inStorage {
		fileName := strings.TrimPrefix(r.URL.Path, "/")
		if fileName == "" {
			fileName = "index.html"
		}
		calcEtag, err := bagEtag(id, fileName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		etag := r.Header.Get("If-None-Match")
		if etag == calcEtag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		bag, err := h.bags.GetBag(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		fileInfo, err := bag.GetFileOffsets(fileName)
		if err != nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		fileType := mime.TypeByExtension(filepath.Ext(fileName))
		if fileType == "" {
			fileType = "application/octet-stream"
		}

		maxRange := fileInfo.Size
		if maxRange > 0 {
			maxRange--
		}
		from, to, hasRange, err := api.ParseRange(r, maxRange)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileInfo.Size))
			http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
			return
		}

		var status int
		if hasRange {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, fileInfo.Size))
			w.Header().Set("Content-Length", fmt.Sprint(to-from+1))
			status = http.StatusPartialContent
		} else {
			w.Header().Set("Content-Length", fmt.Sprint(fileInfo.Size))
			w.Header().Set("Accept-Ranges", "bytes")
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", fileType)
		// todo: is it okay to set this cache control in our case?
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Etag", calcEtag)
		w.WriteHeader(status)
		bag.WriteFileTo(r.Context(), w, fileInfo, from, to, 8)
	} else {
		conn, err := h.rldp.GetConnection(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		queryId := make([]byte, 32)
		rand.Read(queryId)
		rldpReq := &proxy.Request{
			Id:      queryId,
			Method:  r.Method,
			Url:     fmt.Sprintf("http://%s%s", realHost, r.URL.Path),
			Version: "HTTP/1.1",
			Headers: []proxy.Header{
				{Name: "Host", Value: realHost},
			},
		}

		for name, header := range r.Header {
			if _, ok := hopHeaders[name]; ok {
				continue
			}
			for _, value := range header {
				rldpReq.Headers = append(rldpReq.Headers, proxy.Header{
					Name:  name,
					Value: value,
				})
			}
		}

		resp, body, err := conn.SendRequest(r.Context(), rldpReq, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		for _, header := range resp.Headers {
			w.Header().Add(header.Name, header.Value)
		}
		w.WriteHeader(int(resp.StatusCode))
		if resp.NoPayload {
			return
		}
		io.Copy(w, body)
	}
}

func (h *Handler) resolveRealHost(host string) (string, bool) {
	for _, namespace := range h.namespaces {
		i := strings.Index(host, namespace)
		if i < 0 {
			continue
		}
		return host[:i] + strings.ReplaceAll(namespace[:len(namespace)-1], "-", "."), true
	}
	return "", false
}

func (h *Handler) resolve(ctx context.Context, host string) (id []byte, inStorage bool, err error) {
	if adnlAddr, found := strings.CutSuffix(host, ".adnl"); found {
		id, err = api.ParseAdnl(adnlAddr)
		return id, false, err
	}
	if bagId, found := strings.CutSuffix(host, ".bag"); found {
		id, err = hex.DecodeString(bagId)
		return id, true, err
	}
	domain, err := h.dns.Resolve(ctx, host)
	if err != nil {
		return nil, false, err
	}
	id, inStorage = domain.GetSiteRecord()
	if len(id) == 0 {
		return nil, false, fmt.Errorf("no ton site record found for %q", host)
	}
	return id, inStorage, err
}

func bagEtag(bagId []byte, fileName string) (string, error) {
	digest := md5.New()
	if _, err := digest.Write(bagId); err != nil {
		return "", fmt.Errorf("failed to calculate etag: %w", err)
	}
	if _, err := digest.Write([]byte(fileName)); err != nil {
		return "", fmt.Errorf("failed to calculate etag: %w", err)
	}
	// we use metadata for the etag calculation, so the etag should be weak 'W/'
	return fmt.Sprintf("W/\"%x\"", digest.Sum(nil)), nil
}
