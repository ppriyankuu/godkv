
# Distributed Key-Value Store

It starts as a single-node durable store and scales to a multi-node cluster with replication and quorum reads/writes.

## Project layout
```
kvstore/
├── cmd/
│   ├── server/      # HTTP server entrypoint
│   └── client/      # CLI client entrypoint
├── internal/
│   ├── store/       # Core storage: in-memory DB, WAL, snapshots
│   ├── cluster/     # Node/cluster logic, hashing, replication
│   ├── api/         # HTTP handlers & middleware
│   └── client/      # Go client library
├── pkg/             # Shared helpers (reserved)
├── go.mod
└── go.sum

```

## Key Features
### Single-Node
- Concurrency-safe in-memory store using `sync.RWMutex`.
- Write-Ahead Log (WAL) for crash-safe persistence.
- Snapshot creation and automatic restore on startup.
- HTTP API built with Gin: PUT `/kv/:key`, GET `/kv/:key`, DELETE `/kv/:key`.

### Distributed
- Consistent hashing to partition keys across nodes.
- Replication factor `N`: each key is stored on `N` nodes.
- Quorum reads/writes with `R + W > N` rule for strong consistency.
- Vector clocks to track versions and resolve conflicts.
- Read repair for eventual consistency.
- HTTP replication with retries and exponential backoff.
- WAL replication to keep follower logs up to date.

### Cluster Management
- Static configuration for initial cluster members.
- Join/leave endpoints: `/cluster/join`, `/cluster/leave`.

### CLI Client
- Full Go client library.
- Cobra-based CLI with put, get, and delete commands.
- Built-in HTTP timeout and error handling.

## Basic usage
1. Build and run a server node
```
cd cmd/server
go run main.go -config config.json
```
2. Use the CLI client
```
cd cmd/client
go run main.go put mykey myvalue
go run main.go get mykey
go run main.go delete mykey
```
3. Check cluster status
```
curl http://localhost:8080/cluster/status
```

### Disclaimer!!!
```
Parts of this project (and this README!) were built with a little help from AI
``` 
