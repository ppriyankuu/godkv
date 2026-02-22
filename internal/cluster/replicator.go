package cluster

import (
	"bytes"
	"context"
	"distributed-kvstore/internal/store"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// Replicator handles all inter-node communication for reads and writes.
//
// Interview explanation — Quorum reads/writes:
//
//   With N replicas, W write-quorum, and R read-quorum we ensure strong
//   consistency as long as W + R > N.  Classic choice: N=3, W=2, R=2.
//
//   Write path:
//     1. Client sends PUT to the coordinator node.
//     2. Coordinator writes locally.
//     3. Coordinator fans out to all N-1 peers in parallel.
//     4. Once W-1 peers acknowledge (W total including self), return success.
//     5. Remaining peers are updated asynchronously ("hinted handoff").
//
//   Read path:
//     1. Coordinator asks R replicas for the value.
//     2. Compares versions using vector clocks.
//     3. Returns the most recent version.
//     4. If stale replicas are detected, triggers read repair (async write-back).
//
//   This tolerates up to N-W write failures and N-R read failures.

// ReplicaResponse is the result of contacting a single replica.
type ReplicaResponse struct {
	NodeID string
	Value  *store.Value
	Err    error
}

// Replicator fans writes and reads out to replica nodes.
type Replicator struct {
	selfID     string
	membership *Membership
	store      *store.Store
	httpClient *http.Client

	// Quorum parameters
	N int // replication factor
	W int // write quorum
	R int // read quorum
}

// NewReplicator creates a Replicator.  N, W, R must satisfy W+R > N for strong consistency.
func NewReplicator(selfID string, m *Membership, s *store.Store, n, w, r int) *Replicator {
	return &Replicator{
		selfID:     selfID,
		membership: m,
		store:      s,
		N:          n,
		W:          w,
		R:          r,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// ─── Write path ───────────────────────────────────────────────────────────────

// ReplicateWrite writes to W nodes and returns the final stored Value.
func (rep *Replicator) ReplicateWrite(key, data string, clock store.VectorClock) (store.Value, error) {
	// Write locally first — coordinator always participates.
	val, err := rep.store.Put(key, data, clock)
	if err != nil {
		return store.Value{}, fmt.Errorf("local write: %w", err)
	}

	replicas := rep.membership.ReplicaNodes(key, rep.N)
	peers := rep.peersOnly(replicas) // exclude self

	type result struct {
		nodeID string
		err    error
	}
	results := make(chan result, len(peers))

	for _, peer := range peers {
		go func(p *Node) {
			err := rep.sendReplicateRequest(p, key, val)
			results <- result{p.ID, err}
		}(peer)
	}

	// Collect until we reach write quorum (W-1 peers since we already have self).
	acks := 1 // self counts as one ack
	required := rep.W
	var errs []error

	timeout := time.After(5 * time.Second)
	remaining := len(peers)

	for remaining > 0 {
		select {
		case r := <-results:
			remaining--
			if r.err == nil {
				acks++
				if acks >= required {
					return val, nil // quorum reached
				}
			} else {
				errs = append(errs, fmt.Errorf("node %s: %w", r.nodeID, r.err))
			}
		case <-timeout:
			if acks >= required {
				return val, nil
			}
			return store.Value{}, fmt.Errorf("write quorum timeout (%d/%d acks), errors: %v", acks, required, errs)
		}
	}

	if acks >= required {
		return val, nil
	}
	return store.Value{}, fmt.Errorf("write quorum not met (%d/%d), errors: %v", acks, required, errs)
}

// ─── Read path ────────────────────────────────────────────────────────────────

// CoordinateRead fetches from R replicas, reconciles versions, and performs
// async read repair on any stale replicas.
func (rep *Replicator) CoordinateRead(key string) (*store.Value, error) {
	replicas := rep.membership.ReplicaNodes(key, rep.N)

	responses := make(chan ReplicaResponse, len(replicas))

	for _, node := range replicas {
		go func(n *Node) {
			if n.ID == rep.selfID {
				v, ok := rep.store.GetRaw(key)
				if !ok {
					responses <- ReplicaResponse{NodeID: n.ID, Value: nil}
					return
				}
				responses <- ReplicaResponse{NodeID: n.ID, Value: &v}
			} else {
				v, err := rep.fetchFromPeer(n, key)
				responses <- ReplicaResponse{NodeID: n.ID, Value: v, Err: err}
			}
		}(node)
	}

	// Gather R responses.
	var collected []ReplicaResponse
	timeout := time.After(5 * time.Second)
	required := rep.R

	for len(collected) < required {
		select {
		case r := <-responses:
			collected = append(collected, r)
		case <-timeout:
			if len(collected) >= required {
				break
			}
			return nil, fmt.Errorf("read quorum timeout (%d/%d responses)", len(collected), required)
		}
	}

	// Reconcile: find the most recent version by vector clock comparison.
	winner, stale := reconcile(collected)
	if winner == nil {
		return nil, nil // key not found on any replica
	}
	if winner.Tombstone {
		return nil, nil // deleted
	}

	// Read repair: asynchronously bring stale replicas up to date.
	if len(stale) > 0 {
		go rep.readRepair(key, *winner, stale)
	}

	return winner, nil
}

// reconcile picks the most recent Value and lists nodes that are behind.
func reconcile(responses []ReplicaResponse) (winner *store.Value, staleNodes []string) {
	for _, r := range responses {
		if r.Err != nil || r.Value == nil {
			continue
		}
		if winner == nil {
			winner = r.Value
			continue
		}
		rel := r.Value.Clock.Compare(winner.Clock)
		switch rel {
		case store.After:
			staleNodes = append(staleNodes, "") // old winner is stale — but we don't track its node here
			winner = r.Value
		case store.Before:
			staleNodes = append(staleNodes, r.NodeID)
		case store.ConcurrentClocks:
			// Conflict: pick by wall clock as tiebreaker.
			if r.Value.UpdatedAt.After(winner.UpdatedAt) {
				winner = r.Value
			} else {
				staleNodes = append(staleNodes, r.NodeID)
			}
		}
	}
	return winner, staleNodes
}

// readRepair writes the authoritative value back to stale nodes.
// This is how eventual consistency "heals" itself without a background job.
func (rep *Replicator) readRepair(key string, val store.Value, staleNodeIDs []string) {
	for _, id := range staleNodeIDs {
		node, ok := rep.membership.GetNode(id)
		if !ok {
			continue
		}
		_ = rep.sendReplicateRequest(node, key, val) // best-effort
	}
}

// ─── HTTP transport ───────────────────────────────────────────────────────────

// ReplicateRequest is the wire format for replication messages.
type ReplicateRequest struct {
	Key   string      `json:"key"`
	Value store.Value `json:"value"`
}

// sendReplicateRequest sends a value to a peer with exponential backoff retries.
//
// Why exponential backoff?  Thundering-herd prevention.  If a node is briefly
// overloaded and all peers hammer it with retries simultaneously, each retry
// makes the overload worse.  Exponential backoff with jitter spreads the load.
func (rep *Replicator) sendReplicateRequest(peer *Node, key string, val store.Value) error {
	body := ReplicateRequest{Key: key, Value: val}

	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Backoff: 100ms, 200ms, 400ms … with a cap.
			delay := time.Duration(math.Pow(2, float64(attempt-1))*100) * time.Millisecond
			time.Sleep(delay)
		}

		err := rep.doHTTPReplicate(peer, body)
		if err == nil {
			return nil
		}

		if attempt == maxRetries-1 {
			return fmt.Errorf("replicate to %s after %d attempts: %w", peer.ID, maxRetries, err)
		}
	}
	return nil
}

func (rep *Replicator) doHTTPReplicate(peer *Node, body ReplicateRequest) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/internal/replicate", peer.Address)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := rep.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("peer returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// fetchFromPeer GETs the raw value (including tombstones) from a peer node.
func (rep *Replicator) fetchFromPeer(peer *Node, key string) (*store.Value, error) {
	url := fmt.Sprintf("http://%s/internal/fetch/%s", peer.Address, key)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := rep.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("peer returned HTTP %d", resp.StatusCode)
	}

	var val store.Value
	if err := json.NewDecoder(resp.Body).Decode(&val); err != nil {
		return nil, err
	}
	return &val, nil
}

// peersOnly filters the replica list to exclude self.
func (rep *Replicator) peersOnly(nodes []*Node) []*Node {
	var peers []*Node
	for _, n := range nodes {
		if n.ID != rep.selfID {
			peers = append(peers, n)
		}
	}
	return peers
}

// DeleteReplicated replicates a delete (tombstone) to W nodes.
func (rep *Replicator) DeleteReplicated(key string) error {
	if err := rep.store.Delete(key); err != nil {
		return err
	}
	val, _ := rep.store.GetRaw(key) // tombstone value

	replicas := rep.membership.ReplicaNodes(key, rep.N)
	peers := rep.peersOnly(replicas)

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(p *Node) {
			defer wg.Done()
			_ = rep.sendReplicateRequest(p, key, val)
		}(peer)
	}
	wg.Wait()
	return nil
}
