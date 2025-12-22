# oxylume index
indexer service for TON sites. official frontend can be found [here](https://github.com/oxylume/web)

- collects TON domain
- monitors uptime of active TON sites
- provides data about TON sites
- provides TON network gateway using subdomain resolution for domains, bags (.bag) and ADNL (.adnl)

docker image `oxylume/index` is available at [Docker Hub](https://hub.docker.com/r/oxylume/index)

##### support project
if you love this project and want to support its development you can donate on this TON address
`ishoneypot.ton` or `UQA705AUWErQe9Ur56CZz-v6N9J2uw298w-31ZCu475hT8U4`

## prerequisites
- Docker
- Go 1.25+ (for build only)

## usage
### quick start
[download](/docker-compose.yaml) `docker-compose.yaml` file
```bash
wget https://raw.githubusercontent.com/oxylume/mylocalton/refs/heads/main/.env
```
modify [environment variables](#environment-variables) if required (defaults work fine)

start services
```bash
docker compose up
```

access [the api](#endpoints)
```bash
curl http://localhost:8081/sites/stats
```

access TON site via the gateway
```bash
curl http://ishoneypot.ton.localhost:8082
```

## build from sources
build docker image
```bash
docker compose build
```

or build an executable
```bash
go build -o main ./cmd/api
```
the executable depends on `./migrations/` directory to run migrations on start up

## run from sources
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

run the program
```bash
go run ./cmd/api
```

## environment variables
| name             | default | note |
| ---------------- | ------- | ---- |
| `API_LISTEN`     | :8081 | listen address for the api server
| `GATEWAY_LISTEN` | :8082 | listen address for the gateway server
| `DATABASE_URL`   | postgres://postgres@localhost:5432/tonsite?sslmode=disable | postgresql connection url
| `TON_CONFIG_URL` | https://ton.org/global-config.json | json config containing lite servers and dht nodes
| `BAG_TTL`        | 3600 | seconds until evicting stale ton storage bags from a cache (stale means not used for a period of time)
| `CHECK_INTERVAL` | 7200 | seconds until a site need to be checked again
| `DOMAIN_SOURCES` | EQC3dNlesgVD8YbAazcauIrXBPfiVhMMr5YYk2in0Mtsz0Bz;.ton,EQCA14o1-VWhS2efqoh_9M1b_A9DtKTuoqfmkn83AbJzwnPi;.t.me | domain sources must adhere to [TEP-62](https://github.com/ton-blockchain/TEPs/blob/master/text/0062-nft-standard.md) and [TEP-81](https://github.com/ton-blockchain/TEPs/blob/master/text/0081-dns-standard.md). format is comma-separated list of `<collection_address>;<domain_zone>`, domain zone must start with a dot
| `TONCENTER_URL`  | https://toncenter.com/api | toncenter base api url
| `TONCENTER_KEY`  | - | optional toncenter api key [@tonapibot](https://t.me/tonapibot) (without the key you get 1 rps, which is totally ok, but providing the key can slightly speed up the crawling process)

## endpoints
### GET `/sites/stats`
Get statistics about indexed TON sites

**response**
```json
{
    "domains": 420,
    "sites": 69,
    "active": 25
}
```

### GET `/sites/random`
Get data about a random indexed site

**response**
```json
{
    "domain": "ishoneypot.ton",
    "unicode": "ishoneypot.ton",
    "address": "0:7e664d95714bd66e7674afd91087ec42d76c7f3a1861417e6ae1c00313719539",
    "accessible": true,
    "inStorage": false,
    "spamContent": false,
    "checkedUtime": 1765998574
}
```

### GET `/sites`
List filtered data about indexed sites
| query | type | note |
| --- | --- | --- |
| `search` | `string` | search term
| `inaccessible` | `bool` | include inaccessible sites
| `punycode` | `bool?` | show only (`true`) or exclude (`false`) punycode domains
| `spam` | `bool` | include sites with a potentially spam content
| `zone` | `string` | show sites only from a specified domain zone defined by `DOMAIN_SOURCES` env var
| `sort` | `string` | sort field. allowed values:<br> - `domain` (lexicographical)<br> - `checked_at`
| `desc` | `bool` | sort in descending order
| `cursor` | `string` | opaque cursor to list the next batch of sites
| `limit` | `int` | maximum number of sites to return. default `50`. max `1000`

**response**
```json
{
    "sites": [
        {
            "domain": "ishoneypot.ton",
            "unicode": "ishoneypot.ton",
            "address": "0:7e664d95714bd66e7674afd91087ec42d76c7f3a1861417e6ae1c00313719539",
            "accessible": true,
            "inStorage": false,
            "spamContent": false,
            "checkedUtime": 1766013291
        }
    ],
    "cursor": "MDEyMy50b24="
}
```
