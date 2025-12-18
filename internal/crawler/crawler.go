package crawler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oxylume/index/internal/db"
	"github.com/oxylume/index/pkg/api/toncenter"
	"github.com/oxylume/index/pkg/proxy"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/ton/dns"
	"golang.org/x/net/idna"
)

const (
	noNewDelay = 10 * time.Second
)

type DomainSource struct {
	Address *address.Address
	Zone    string
}

type Crawler struct {
	dns       *dns.Client
	bags      *proxy.BagProvider
	rldp      *proxy.RLDPConnector
	sites     *db.SitesStore
	state     *db.CrawlerStore
	toncenter *toncenter.Client
	closer    context.CancelFunc
}

func NewCrawler(dns *dns.Client, bags *proxy.BagProvider, rldp *proxy.RLDPConnector, sites *db.SitesStore, state *db.CrawlerStore, toncenter *toncenter.Client) *Crawler {
	return &Crawler{
		dns:       dns,
		bags:      bags,
		rldp:      rldp,
		sites:     sites,
		state:     state,
		toncenter: toncenter,
	}
}

func (c *Crawler) Start(ctx context.Context, sources []*DomainSource) error {
	ctx, c.closer = context.WithCancel(ctx)
	for _, src := range sources {
		offset, err := c.state.GetOffset(ctx, src.Address.StringRaw())
		if err != nil {
			c.closer()
			return fmt.Errorf("unable to get crawler offset for %s: %w", src.Address.String(), err)
		}
		go c.worker(ctx, src, offset)
	}
	return nil
}

func (c *Crawler) Close() {
	if c.closer != nil {
		c.closer()
	}
}

func (c *Crawler) worker(ctx context.Context, src *DomainSource, offset int) {
	const limit = 500
	srcAddr := src.Address.StringRaw()
	for {
		if ctx.Err() != nil {
			return
		}

		nfts, err := c.toncenter.GetNftsByCollection(ctx, srcAddr, limit, offset)
		if err != nil {
			continue
		}
		if len(nfts) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(noNewDelay):
			}
			continue
		}
		sites := make([]db.SiteCreate, 0, len(nfts))
		for _, nft := range nfts {
			domain := nft.Content.Domain
			if domain == "" {
				log.Printf("[CRAWLER] nft %s is missing a domain", nft.Address)
				continue
			}
			addr, err := address.ParseRawAddr(nft.Address)
			if err != nil {
				log.Printf("[CRAWLER] unable to parse address %s", addr)
				continue
			}
			unicode, err := idna.Punycode.ToUnicode(domain)
			if err != nil {
				log.Printf("[CRAWLER] unable to convert %s to unicode form", domain)
				unicode = domain
			}
			sites = append(sites, db.SiteCreate{
				Domain:  domain,
				Unicode: unicode,
				Zone:    src.Zone,
				Address: addr.StringRaw(),
			})
		}
		if err := c.sites.AddDomains(ctx, sites...); err != nil {
			log.Printf("[CRAWLER] unable to register domains: %v", err)
			continue
		}
		if err := c.state.SetOffset(ctx, srcAddr, offset+len(nfts)); err != nil {
			log.Printf("[CRAWLER] unable to save offset: %v", err)
			continue
		}
		offset += len(nfts)
	}
}
