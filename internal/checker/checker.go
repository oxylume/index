package checker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/oxylume/index/internal/db"
	"github.com/oxylume/index/pkg/proxy"
	"github.com/xssnick/tonutils-go/ton/dns"
)

const sniffSize = 512
const timeout = 16 * time.Second
const hold = timeout + timeout/4

var errSpamContent = errors.New("contains spam content")

type Checker struct {
	dns    *dns.Client
	bags   *proxy.BagProvider
	rldp   *proxy.RLDPConnector
	sites  *db.SitesStore
	stale  time.Duration
	closer context.CancelFunc
}

func NewChecker(dns *dns.Client, bags *proxy.BagProvider, rldp *proxy.RLDPConnector, sites *db.SitesStore, stale time.Duration) *Checker {
	return &Checker{
		dns:   dns,
		bags:  bags,
		rldp:  rldp,
		sites: sites,
		stale: stale,
	}
}

func (c *Checker) Start(ctx context.Context, workers int) {
	ctx, c.closer = context.WithCancel(ctx)
	domainsC := make(chan string, workers)
	go c.reserver(ctx, domainsC, workers)
	for range workers {
		go c.worker(ctx, domainsC)
	}
}

func (c *Checker) Close() {
	if c.closer != nil {
		c.closer()
	}
}

func (c *Checker) worker(ctx context.Context, domainsC <-chan string) {
	for domain := range domainsC {
		if ctx.Err() != nil {
			return
		}
		status, inStorage, spamContent := c.check(ctx, domain)
		if err := c.sites.FinalizeCheck(ctx, domain, status, inStorage, spamContent); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("[CHECKER] unable to update site status: %v", err)
			}
			continue
		}
	}
}

func (c *Checker) reserver(ctx context.Context, domainsC chan<- string, reserveBatch int) {
	defer close(domainsC)
	for {
		if ctx.Err() != nil {
			return
		}
		sites, err := c.sites.ReserveCheck(ctx, c.stale, hold, reserveBatch)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("[CHECKER]: failed to get expired sites: %v", err)
			}
			continue
		}
		if len(sites) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}
		for _, site := range sites {
			select {
			case <-ctx.Done():
				return
			case domainsC <- site:
			}
		}
	}
}

// todo: it would be better to implement a block scanner to detect "TON Site" record changes
// rather than asking each domain separately, especially long-term
func (c *Checker) check(ctx context.Context, domain string) (db.SiteStatus, bool, bool) {
	resolved, err := c.dns.Resolve(ctx, domain)
	if err != nil {
		return db.StatusNoSite, false, false
	}
	id, inStorage := resolved.GetSiteRecord()
	if id == nil {
		return db.StatusNoSite, false, false
	}
	data, err := c.getSiteData(ctx, domain, id, inStorage)
	if err != nil {
		return db.StatusInaccessible, inStorage, false
	}
	return db.StatusAccessible, inStorage, containsSpamContent(data)
}

// func (c *Checker) validateContent(data []byte) error {
// 	mimeType := http.DetectContentType(data)
// 	if !strings.HasPrefix(mimeType, "text/html") {
// 		return fmt.Errorf("invalid mime %s", mimeType)
// 	}
// 	return nil
// }

func (c *Checker) getSiteData(ctx context.Context, domain string, id []byte, inStorage bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var data []byte
	if inStorage {
		bag, err := c.bags.GetBag(ctx, id)
		if err != nil {
			return nil, err
		}
		info, err := bag.GetFileOffsets("index.html")
		if err != nil {
			return nil, err
		}
		if info.Size == 0 {
			return nil, fmt.Errorf("empty file")
		}
		size := min(info.Size, sniffSize)

		buf := bytes.NewBuffer(make([]byte, 0, size))
		bag.WriteFileTo(ctx, buf, info, 0, size-1, 1)
		data = buf.Bytes()
	} else {
		conn, err := c.rldp.GetConnection(ctx, id)
		if err != nil {
			return nil, err
		}
		req := &proxy.Request{
			Method:  "GET",
			Url:     fmt.Sprintf("http://%s", domain),
			Version: "HTTP/1.1",
			Headers: []proxy.Header{
				{Name: "Host", Value: domain},
			},
		}
		resp, body, err := conn.SendRequest(ctx, req, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("responded with non-ok status code %d", resp.StatusCode)
		}
		if resp.NoPayload {
			return nil, fmt.Errorf("responded with empty payload")
		}
		data = make([]byte, sniffSize)
		_, err = body.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	}
	return data, nil
}
