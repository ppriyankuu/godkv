# Distributed Key-Value Store

A production-grade distributed KV store written in Go, implementing:
- Consistent hashing
- Quorum-based replication
- Vector clocks
- WAL-backed persistence
- Read repair

---

## Quick Start

```bash
# Install dependencies
go mod tidy

# Start a single node
go run ./cmd/server --id node1 --addr :8080 --data-dir /tmp/kvstore

# Start a 3-node cluster (3 separate terminals)
go run ./cmd/server --id node1 --addr :8080 --data-dir /tmp/kv \
    --peers node2=localhost:8081,node3=localhost:8082 --n 3 --w 2 --r 2

go run ./cmd/server --id node2 --addr :8081 --data-dir /tmp/kv \
    --peers node1=localhost:8080,node3=localhost:8082 --n 3 --w 2 --r 2

go run ./cmd/server --id node3 --addr :8082 --data-dir /tmp/kv \
    --peers node1=localhost:8080,node2=localhost:8081 --n 3 --w 2 --r 2

# Use the CLI
go run ./cmd/client put hello "world" --server http://localhost:8080
go run ./cmd/client get hello --server http://localhost:8080
go run ./cmd/client delete hello --server http://localhost:8080
go run ./cmd/client cluster nodes --server http://localhost:8080
```

---

## Basic Architecture

```
Client (CLI / Go library)
          │ HTTP
          ▼
  Coordinator Node
     ├── Gin Router
     ├── Replicator (quorum + repair)
     └── Store (WAL + snapshot)
          │
          ▼
   Replica Nodes
```

Each request is handled by a **coordinator** node, which:
1. Determines replica nodes using consistent hashing.
2. Executes quorum read/write logic.
3. Stores data locally using WAL-backed persistence.
4. Performs read repair when inconsistencies are detected.

---

## Folder Structure
```
kvstore/
├── README.md
├── go.mod
│
├── cmd/
│   ├── server/
│   │   └── main.go              # Node entrypoint, flags, graceful shutdown
│   └── client/
│       └── main.go              # Cobra CLI (put / get / delete / cluster)
│
└── internal/
    ├── store/
    │   ├── store.go             # In-memory map, Put/Get/Delete, snapshot logic
    │   ├── wal.go               # Write-Ahead Log (append-only NDJSON)
    │   └── vector_clock.go      # Vector clock comparison & merge
    │
    ├── cluster/
    │   ├── ring.go              # Consistent hash ring with virtual nodes
    │   ├── membership.go        # Node join/leave, replica node lookup
    │   └── replicator.go        # Quorum writes/reads, read repair, backoff
    │
    ├── api/
    │   ├── handlers.go          # Gin HTTP handlers (public + internal routes)
    │   └── middleware.go        # Request logger, panic recovery
    │
    └── client/
        ├── client.go            # Typed Go client library (Put/Get/Delete)
        └── raw.go               # Raw HTTP helper for misc endpoints
```

---

## Component Deep-Dives (Interview Notes)

### 1. Write-Ahead Log (WAL) — `internal/store/wal.go`

Every write is appended to an on-disk log **before** being applied to the
in-memory map.  If the process crashes mid-write, on restart we replay the
WAL from top to bottom to recover exactly the state that existed before the
crash.

- Entries are newline-delimited JSON (easy to inspect, easy to parse).
- Each append calls `fsync` to force OS buffers to physical media.
- Snapshots compress history: after a snapshot the WAL is truncated.

**Key interview point:** WAL entries must be idempotent.  Re-applying a PUT
twice should produce the same result.  Our vector clock comparison in
`ApplyRemote` handles this: an older entry won't overwrite a newer one.

---

### 2. Consistent Hashing — `internal/cluster/ring.go`

Keys are mapped to nodes using a hash ring:

1. Each physical node is placed at `vnodes` (default 150) positions on a
   2³² ring by hashing `"nodeID#0"`, `"nodeID#1"`, … .
2. A key is owned by the first node **clockwise** from `hash(key)`.
3. For replication factor N, we walk clockwise and pick the first N distinct
   physical nodes.

Adding/removing a node only remaps keys between that node and its predecessor
(≈ 1/N of all keys), versus naive modular hashing which remaps nearly everything.

Virtual nodes solve the **load balance** problem: without them, unlucky hash
placement could give one node 50% of the ring.  150 vnodes per node gives a
standard deviation of ~10% in load.

---

### 3. Vector Clocks — `internal/store/vector_clock.go`

A vector clock is `map[nodeID]→counter`.  Every write increments the
writer's own counter.  When two replicas hold different values for the
same key, we compare their clocks:

| Relationship | Meaning |
|---|---|
| A < B | A happened-before B; discard A |
| A > B | B happened-before A; discard B |
| A ∥ B | Concurrent writes — true conflict |

For conflicts we fall back to **wall-clock last-write-wins** (pragmatic; used
by Cassandra/Riak).  A production system could instead surface the conflict to
the application.

---

### 4. Quorum Reads/Writes — `internal/cluster/replicator.go`

With N replicas, write quorum W, and read quorum R, strong consistency holds
when **W + R > N**.  Classic choice: N=3, W=2, R=2.

**Write path:**
1. Coordinator writes locally (counts as 1 ack).
2. Fans out to all N-1 peers in parallel goroutines.
3. Returns success once W total acks received.
4. Remaining peers catch up asynchronously.

**Read path:**
1. Coordinator asks R replicas for their version.
2. Reconciles using vector clocks to pick the winner.
3. Any stale replica is updated via **read repair** (async write-back).

This tolerates up to `N-W` write failures and `N-R` read failures.

---

### 5. Read Repair — `internal/cluster/replicator.go`

When a read discovers that some replicas have a stale version, the coordinator
**asynchronously writes the authoritative version back** to those replicas.
This is how Cassandra achieves eventual consistency without a background job.

The repair is fire-and-forget (best effort) — if the repair fails, the stale
replica will get corrected on the next successful read.

---

### 6. Snapshots — `internal/store/store.go`

Without snapshots, recovering from a crash requires replaying the entire WAL —
unbounded and slow.  Snapshots:

1. Serialize the in-memory map to `snapshot.json` via an atomic write
   (write to `.tmp`, then `os.Rename` — crash-safe).
2. Truncate the WAL (everything is captured in the snapshot).
3. On startup: load snapshot, then replay only WAL entries written **after**
   the snapshot.

Snapshots are taken automatically every 60 seconds in the background goroutine
in `cmd/server/main.go`, and also on graceful shutdown.

---

## API Reference

| Method | Path | Description |
|---|---|---|
| `GET` | `/kv/:key` | Read a value (quorum read) |
| `PUT` | `/kv/:key` | Write a value (quorum write). Body: `{"value":"…"}` |
| `DELETE` | `/kv/:key` | Delete a value (tombstone + replicate) |
| `GET` | `/cluster/nodes` | List all cluster members |
| `POST` | `/cluster/join` | Add a node. Body: `{"id":"…","address":"…"}` |
| `POST` | `/cluster/leave` | Remove a node. Body: `{"id":"…"}` |
| `GET` | `/health` | Health check |
| `POST` | `/internal/replicate` | Peer replication endpoint |
| `GET` | `/internal/fetch/:key` | Peer raw-fetch endpoint (for read repair) |
