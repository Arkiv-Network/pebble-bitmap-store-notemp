# Pebble Bitmap Store

A Go-based system for efficiently storing, indexing, and querying blockchain event data from the Arkiv Network. Uses PebbleDB for persistence and Roaring Bitmaps for fast attribute-based filtering.

## Features

- **Bitmap Indexing**: Uses Roaring Bitmap compression for memory-efficient attribute indexes
- **Custom Query Language**: Boolean expressions with comparisons, glob patterns, set operations (`in`/`not in`), and keyword alternatives (`AND`/`OR`/`NOT`)
- **Blockchain Event Processing**: Handles Create, Update, Delete, Expire, ExtendBTL, and ChangeOwner operations
- **Synthetic Attributes**: Automatic indexing of `$owner`, `$creator`, `$key`, `$expiration`, `$createdAtBlock`, `$lastModifiedAtBlock`, `$sequence`, `$txIndex`, `$opIndex`
- **PebbleDB Storage**: Key-value store with prefix-based organization, Snappy/Zstd compression, and bloom filters
- **Paginated Queries**: Cursor-based pagination with configurable page size (max 200 results)
- **Snapshot Reads**: Queries use PebbleDB snapshots for consistent point-in-time reads

## Usage

### Loading Data

Load blockchain events from a TAR archive:

```bash
go run ./cmd/load-from-tar --db-path arkiv-data.db <tar-file>

# Or using environment variable
DB_PATH=arkiv-data.db go run ./cmd/load-from-tar <tar-file>
```

### Querying

Query the database with filter expressions:

```bash
go run ./cmd/query --db-path arkiv-data.db 'type = "thing"'
go run ./cmd/query --db-path arkiv-data.db '$owner = "0x1234..."'
```

## Query Language

### Operators

| Operator | Description |
|----------|-------------|
| `&&`, `AND` | Logical AND |
| `\|\|`, `OR` | Logical OR |
| `!`, `NOT` | Logical NOT |
| `=`, `!=` | Equality comparison |
| `<`, `>`, `<=`, `>=` | Numeric comparison |
| `~`, `GLOB` | Glob pattern match |
| `!~`, `NOT GLOB` | Glob pattern not match |
| `in` | Set inclusion |
| `not in` | Set exclusion |

### Special Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `$owner` | string | Entity owner address |
| `$creator` | string | Entity creator address |
| `$key` | string | Entity key |
| `$expiration` | numeric | Expiration block number |
| `$createdAtBlock` | numeric | Block where entity was created |
| `$lastModifiedAtBlock` | numeric | Block of last modification |
| `$sequence` | numeric | Composite: (blockNum << 32) \| (txIndex << 16) \| opIndex |
| `$txIndex` | numeric | Transaction index in block (stored, not bitmap-indexed) |
| `$opIndex` | numeric | Operation index in transaction (stored, not bitmap-indexed) |
| `$all` | - | Match all entities |
| `*` | - | Wildcard (match all) |

### Examples

```
type = "nft" && status = "active"
$owner = "0xabc..." || $creator = "0xabc..."
name ~ "test*" && !(status = "deleted")
price >= 100 && price <= 1000
type in ("nft" "token")
$createdAtBlock > 1000
```

## Storage Layout

Uses PebbleDB with prefix-based key organization:

| Prefix | Description |
|--------|-------------|
| `0x01` | Last processed block number |
| `0x02` | Payload records (prefix + 8-byte big-endian ID) |
| `0x03` | Entity current pointer (32-byte entity key -> ID) |
| `0x06` | ID counter for unique ID allocation |
| `0x10` | String attribute bitmap indexes |
| `0x20` | Numeric attribute bitmap indexes |

## Dependencies

| Package | Purpose |
|---------|---------|
| [pebble](https://github.com/cockroachdb/pebble) | Key-value storage engine |
| [roaring/v2](https://github.com/RoaringBitmap/roaring) | Bitmap compression and operations |
| [participle/v2](https://github.com/alecthomas/participle) | Query language parser |
| [arkiv-events](https://github.com/Arkiv-Network/arkiv-events) | Blockchain event structures |
| [go-ethereum](https://github.com/ethereum/go-ethereum) | Address and Hash types |
| [urfave/cli/v2](https://github.com/urfave/cli) | CLI framework |

## Development

Uses Nix flakes for reproducible development:

```bash
# Enter development shell
nix develop

# Or with direnv
direnv allow
```

Run tests:

```bash
go test ./...
```
