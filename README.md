# tommedag

HTTP JSON service that returns household waste pickup days for an address in
the [Avfall Sør](https://avfallsor.no) region (Kristiansand / Vennesla,
Norway).

Give it a street address; it resolves the address to Avfall Sør's internal
property id and returns the upcoming pickup schedule as clean JSON.

The repository ships two entrypoints over the same core logic:

- **`cmd/server`** — an HTTP JSON API (binary `tommedag`).
- **`cmd/cli`** — a one-shot command-line tool (binary `tommedag-cli`).

## How it works

Avfall Sør's "Finn hentedag" page is backed by two public WordPress REST
endpoints. This service uses them directly (no HTML scraping):

1. **Address search** — resolves free text to a property UUID:

   ```
   GET https://avfallsor.no/wp-json/addresses/v1/address?lookup_term=<address>
   ```

   Returns candidate addresses; the property UUID is embedded in each
   result's `href`.

2. **Pickup calendar** — returns the schedule for a property UUID:

   ```
   GET https://avfallsor.no/wp-json/pickup-calendar/v1/collections/property-id/<uuid>
   ```

`tommedag` combines the two: it searches for the address, auto-picks the best
matching property, fetches that property's calendar, and projects it into a
stable, English-keyed JSON shape.

### Address matching

The upstream search often returns several sub-addresses for one street number
(e.g. `Dronningens gate 24` also matches `24 A`, `24 B`, `24 C Seksj.1–4`,
`24 D`). The service auto-resolves this:

1. Exact (case/whitespace-insensitive) match on the candidate address wins.
2. Otherwise the shortest candidate that the query is a prefix of wins
   (prefers the base address over its sub-units).
3. Otherwise the first result from upstream is used.

The response always includes `ambiguous` and `alternatives_count` so callers
can tell when the pick was a guess. Street-level results with no specific
property (empty `href`) are discarded.

## HTTP API

### `GET /api/v1/pickup-days?address=<address>`

```
curl 'http://localhost:8080/api/v1/pickup-days?address=Dronningens%20gate%2033'
```

```json
{
  "query": "Dronningens gate 33",
  "matched_address": "Dronningens Gate 33, Kristiansand",
  "property_id": "bdd61d75-e427-46b2-bdfc-1962dacaf0ba",
  "ambiguous": false,
  "alternatives_count": 0,
  "collection_year": 2026,
  "has_next_year": false,
  "cutoff_date": "2026-08-31",
  "pickup_days": [
    {
      "date": "2026-07-13",
      "label": "Mandag 13. juli",
      "waste_types": [
        {
          "fraksjon": "Restavfall",
          "fraksjon_id": "9011",
          "on_request": false,
          "frequency": "Kan tømmes mandager hver uke",
          "icons": ["residual"]
        }
      ]
    }
  ]
}
```

Status codes:

| Status | Meaning                                              |
| ------ | --------------------------------------------------- |
| 200    | Address resolved, schedule returned                 |
| 400    | Missing/blank `address` query parameter             |
| 404    | No matching address found                           |
| 502    | Upstream avfallsor.no unavailable or returned junk  |

### `GET /healthz`

Liveness probe. Returns `200 {"status":"ok"}`.

## CLI

`tommedag-cli` performs the same lookup as the HTTP API but as a one-shot
command, printing the JSON response to stdout.

```sh
# Install
go install github.com/sollie/tommedag/cmd/cli@latest

# Or run from the source tree
cd source && go run ./cmd/cli Dronningens gate 33
```

The address is taken from the positional arguments, so no quoting is needed:

```sh
tommedag-cli Dronningens gate 33
```

Flags:

| Flag         | Default | Description                                        |
| ------------ | ------- | -------------------------------------------------- |
| `-timeout`   | `10s`   | Upstream request timeout                           |
| `-compact`   | `false` | Emit compact single-line JSON (for piping)         |
| `-base-url`  | —       | Override upstream base URL (advanced/testing)      |

Output contract: on success the JSON schedule is written to **stdout** and the
process exits `0`. On failure a human-readable message is written to
**stderr** and stdout stays empty, so piping stdout is always safe:

```sh
tommedag-cli -compact Dronningens gate 33 | jq '.pickup_days[0]'
```

Exit codes: `0` success, `1` lookup/upstream error, `2` usage error.

## Configuration

All configuration is via environment variables:

| Variable                    | Default | Description                                            |
| --------------------------- | ------- | ------------------------------------------------------ |
| `TOMMEDAG_ADDR`             | `:8080` | Listen address                                         |
| `TOMMEDAG_CACHE_TTL`        | `1h`    | Pickup-calendar cache TTL (`0` disables). Matches upstream `Cache-Control`. |
| `TOMMEDAG_UPSTREAM_TIMEOUT` | `10s`   | Per-request timeout for calls to avfallsor.no          |

Durations accept Go duration strings (`500ms`, `1h`); a bare integer is
treated as seconds.

## Development

The Go module lives under [`source/`](source/).

```sh
cd source
go test ./... -race -cover
go run ./cmd/server        # HTTP server
go run ./cmd/cli <address> # CLI
```

## Docker

Multi-stage build producing a `scratch` image (CA certificates and zoneinfo
included, runs as non-root UID 65532):

```sh
docker build -t tommedag .
docker run --rm -p 8080:8080 tommedag
```

## Disclaimer

Unofficial. Uses Avfall Sør's public website endpoints, which may change
without notice. Not affiliated with or endorsed by Avfall Sør.
