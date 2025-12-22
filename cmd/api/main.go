package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oxylume/index/cmd/api/config"
	"github.com/oxylume/index/internal/api/handler"
	"github.com/oxylume/index/internal/checker"
	"github.com/oxylume/index/internal/crawler"
	"github.com/oxylume/index/internal/db"
	"github.com/oxylume/index/pkg/api/toncenter"
	"github.com/oxylume/index/pkg/proxy"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/dns"
	"github.com/xssnick/tonutils-storage/storage"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func must1[T any](val T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return val
}

func must2[T1 any, T2 any](val1 T1, val2 T2, err error) (T1, T2) {
	if err != nil {
		log.Fatal(err)
	}
	return val1, val2
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg := must1(config.LoadConfig())
	threads := min(runtime.NumCPU(), 32)

	must(db.RunMigrations("./migrations", cfg.DatabaseUrl))

	tonCfg := must1(liteclient.GetConfigFromUrl(ctx, cfg.TonConfigUrl))

	tonConn := liteclient.NewConnectionPool()
	defer tonConn.Stop()
	must(tonConn.AddConnectionsFromConfig(ctx, tonCfg))
	tonClient := ton.NewAPIClient(tonConn).WithRetry(5)

	root := must1(dns.GetRootContractAddr(ctx, tonClient))
	dnsClient := dns.NewDNSClient(tonClient, root)

	listener := must1(adnl.DefaultListener(":"))
	netManager := adnl.NewMultiNetReader(listener)
	defer netManager.Close()

	_, dhtKey := must2(ed25519.GenerateKey(nil))
	dhtGateway := adnl.NewGatewayWithNetManager(dhtKey, netManager)
	must(dhtGateway.StartClient(threads))
	defer dhtGateway.Close()
	dhtClient := must1(dht.NewClientFromConfig(dhtGateway, tonCfg))
	defer dhtClient.Close()

	_, storageKey := must2(ed25519.GenerateKey(nil))
	storageGateway := adnl.NewGatewayWithNetManager(storageKey, netManager)
	must(storageGateway.StartClient(threads))
	defer storageGateway.Close()
	storageServer := storage.NewServer(dhtClient, storageGateway, storageKey, false, 1)
	connector := storage.NewConnector(storageServer)
	defer storageServer.Stop()
	storage := proxy.NewMemoryStorage()
	storageServer.SetStorage(storage)
	bags := proxy.NewBagProvider(connector, storage, cfg.BagTTL)
	bags.Start(ctx)
	defer bags.Close()

	_, proxyKey := must2(ed25519.GenerateKey(nil))
	proxyGateway := adnl.NewGatewayWithNetManager(proxyKey, netManager)
	must(proxyGateway.StartClient(threads))
	defer proxyGateway.Close()
	rldp := proxy.NewRLDPConnector(proxyGateway, dhtClient)

	dbPool := must1(pgxpool.New(ctx, cfg.DatabaseUrl))
	must(dbPool.Ping(ctx))
	sites := db.NewSitesStore(dbPool)
	crawlerState := db.NewCrawlerStore(dbPool)

	tcClient := toncenter.NewClient(cfg.ToncenterUrl, cfg.ToncenterKey)

	crawler := crawler.NewCrawler(dnsClient, bags, rldp, sites, crawlerState, tcClient)
	crawler.Start(ctx, cfg.DomainSources)
	defer crawler.Close()
	checker := checker.NewChecker(dnsClient, bags, rldp, sites, 2*time.Hour)
	checker.Start(ctx, 100)
	defer checker.Close()

	zones := make([]string, len(cfg.DomainSources))
	for i, src := range cfg.DomainSources {
		zones[i] = src.Zone
	}
	handler := handler.NewHandler(dnsClient, bags, rldp, sites, zones)

	mux := http.NewServeMux()

	apiServer := &http.Server{
		Addr:    cfg.ApiListen,
		Handler: handler.ApiHandler(mux),
	}
	go func() {
		log.Printf("[SERVER] api is now listening on %s", apiServer.Addr) // actually no but im too lazy to separate listen and serve
		if err := apiServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[SERVER] fatal error: %v", err)
		}
	}()
	gatewayServer := &http.Server{
		Addr:    cfg.GatewayListen,
		Handler: handler.GatewayHandler(),
	}
	go func() {
		log.Printf("[SERVER] gateway is now listening on %s", gatewayServer.Addr)
		if err := gatewayServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[SERVER] fatal error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("[SERVER] shutting down...")
	shutdownCtx, shutdownCtxStop := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCtxStop()
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("[SERVER] unable to gracefully shut down api server: %v", err)
		}
	})
	wg.Go(func() {
		if err := gatewayServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("[SERVER] unable to gracefully shut down gateway server: %v", err)
		}
	})
	wg.Wait()
	log.Printf("[SERVER] bye bye")
}
