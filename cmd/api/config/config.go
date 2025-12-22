package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oxylume/index/internal/crawler"
	"github.com/xssnick/tonutils-go/address"
)

const defaultBagTTL = 3600 // 1 hour
// todo: automatically resolve domain zone of passed source
var defaultDomainSrc = []string{
	"EQC3dNlesgVD8YbAazcauIrXBPfiVhMMr5YYk2in0Mtsz0Bz;.ton",  // .ton dns
	"EQCA14o1-VWhS2efqoh_9M1b_A9DtKTuoqfmkn83AbJzwnPi;.t.me", // .t.me dns
}

type Config struct {
	ApiListen     string
	GatewayListen string
	TonConfigUrl  string
	BagTTL        time.Duration
	DatabaseUrl   string
	ToncenterUrl  string
	ToncenterKey  string
	DomainSources []*crawler.DomainSource
}

func LoadConfig() (*Config, error) {
	sourcesRaw := getEnvMany("DOMAIN_SOURCES", defaultDomainSrc...)
	sources := make([]*crawler.DomainSource, len(sourcesRaw))
	for i, raw := range sourcesRaw {
		rawAddr, zone, ok := strings.Cut(raw, ";")
		if !ok {
			return nil, fmt.Errorf("unexpected DOMAIN_SOURCES item format %s, must be <address>;<zone>", raw)
		}
		if !strings.HasPrefix(zone, ".") {
			return nil, fmt.Errorf("DOMAIN_SOURCES zone must begin with a \".\", got %q", zone)
		}
		addr, err := parseAddress(rawAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid DOMAIN_SOURCES address %s: %w", rawAddr, err)
		}
		sources[i] = &crawler.DomainSource{
			Address: addr,
			Zone:    zone,
		}
	}
	return &Config{
		ApiListen:     getEnv("API_LISTEN", ":8081"),
		GatewayListen: getEnv("GATEWAY_LISTEN", ":8082"),
		TonConfigUrl:  getEnv("TON_CONFIG_URL", "https://ton.org/global-config.json"),
		BagTTL:        time.Duration(getEnvInt("BAG_TTL", defaultBagTTL)) * time.Second,
		DatabaseUrl:   getEnv("DATABASE_URL", "postgres://postgres@localhost:5432/tonsite?sslmode=disable"),
		ToncenterUrl:  getEnv("TONCENTER_URL", "https://toncenter.com/api"),
		ToncenterKey:  getEnv("TONCENTER_KEY", ""),
		DomainSources: sources,
	}, nil
}

func getEnv(key string, defaultValue string) string {
	env, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	return env
}

func getEnvMany(key string, defaultValue ...string) []string {
	env, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	if env == "" {
		return nil
	}
	return strings.Split(env, ",")
}

func getEnvInt(key string, defaultValue int) int {
	env, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	val, err := strconv.Atoi(env)
	if err != nil {
		log.Fatalf("invalid integer value for env %s: %v", key, err)
	}
	return val
}

func parseAddress(addr string) (*address.Address, error) {
	parsed, err := address.ParseAddr(addr)
	if err != nil {
		if parsed, err = address.ParseRawAddr(addr); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}
