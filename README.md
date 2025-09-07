# oxylume index
indexer service for TON sites. web frontend can be found [here](https://github.com/oxylume/web)

- collects TON domain
- monitors uptime of active TON sites
- provides data about TON sites
- provides TON network gateway using subdomain resolution for domains (.ton, .t.me and etc), bags (.bag) and ADNL (.adnl)

docker image `oxylume/index` is available at [Docker Hub](https://hub.docker.com/r/oxylume/index)

##### support project
if you love this project and want to support its development you can donate on this TON address
`ishoneypot.ton` or `UQA705AUWErQe9Ur56CZz-v6N9J2uw298w-31ZCu475hT8U4`

## quick start
create `docker-compose.yaml` file

change [environment variables](#environment-variables) if required (defaults work fine)
```yaml
services:
  index:
    image: oxylume/index:latest
    restart: unless-stopped
    ports:
      - 80:8081
    environment:
      DATABASE_URL: postgres://postgres@db:5432/tonsite?sslmode=disable
    depends_on:
      db:
        condition: service_healthy
        restart: true
  db:
    image: postgres:18
    restart: unless-stopped
    environment:
      POSTGRES_HOST_AUTH_METHOD: trust
      POSTGRES_DB: tonsite
    volumes:
      - ./postgres:/var/lib/postgresql
    healthcheck:
      test: pg_isready -d tonsite -U postgres
      interval: 10s
      timeout: 5s
      retries: 5
```

start services
```bash
docker compose up
```

### setup & run
#### prerequisites
- Go 1.25+
- Docker

#### start db
start postgresql database using docker (or do it without docker but that's on you)
```bash
docker run --rm \
    -p 127.0.0.1:5432:5432 \
    -v ./temp/postgres:/var/lib/postgresql \
    -e POSTGRES_HOST_AUTH_METHOD=trust \
    -e POSTGRES_DB=tonsite \
    postgres:18
```
database migrations are stored in the `./migrations/` directory and run by the app on start up

#### start app
run as package
```bash
go run ./cmd/api
```

or build an executable
```bash
go build -o main ./cmd/api
```

## environment variables
| name | default | note |
| --- | --- | --- |
| `BIND_ADDRESS` | :8081 | listen address to accept incoming http requests
| `DATABASE_URL` | postgres://postgres@localhost:5432/tonsite?sslmode=disable | postgresql connection url
| `TON_CONFIG_URL` | https://ton.org/global-config.json | json config containing lite servers and dht nodes
| `BAG_TTL` | 3600 | seconds until evicting stale ton storage bag from a cache (stale means not used for a period of time)
| `GATEWAY_ENABLED` | 1 | enables gateway to serve TON network resources using subdomain resolution. setting it to `0` will disable the gateway
| `DOMAIN_SOURCES` | EQC3dNlesgVD8YbAazcauIrXBPfiVhMMr5YYk2in0Mtsz0Bz;.ton,EQCA14o1-VWhS2efqoh_9M1b_A9DtKTuoqfmkn83AbJzwnPi;.t.me | domain sources must adhere to [TEP-62](https://github.com/ton-blockchain/TEPs/blob/master/text/0062-nft-standard.md) and [TEP-81](https://github.com/ton-blockchain/TEPs/blob/master/text/0081-dns-standard.md). format is comma-separated list of `<collection_address>;<domain_zone>`, domain zone must start with a dot
| `TONCENTER_URL` | https://toncenter.com/api | toncenter base api url
| `TONCENTER_KEY` | - | optional toncenter api key [@tonapibot](https://t.me/tonapibot) (without the key you have 1 rps which is totally ok but providing the key can slightly speed up crawling process)

## endpoints
TBA
