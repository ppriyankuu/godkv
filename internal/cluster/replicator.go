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

////////////////////////////////////////////////////////////////////////////////
// REPLICATOR
////////////////////////////////////////////////////////////////////////////////

// Replicator handles communication between nodes.
//
// In a distributed system, one node acts as the coordinator
// for a request. That node must:
//
//   • Write locally
//   • Send the write to other replicas
//   • Wait for enough acknowledgements (quorum)
//   • Return success or failure
//
// This struct implements quorum reads and quorum writes.
//
// -----------------------------------------------------------------------------
// QUORUM THEORY (Interview explanation)
//
// If:
//   N = total replicas
//   W = write quorum
//   R = read quorum
//
// To guarantee strong consistency:
//
//      W + R > N
//
// Example:
//   N = 3
//   W = 2
//   R = 2
//
// That means:
//   - A write must succeed on at least 2 nodes.
//   - A read must consult at least 2 nodes.
//   - There will always be overlap between read & write nodes.
//
////////////////////////////////////////////////////////////////////////////////

// ReplicaResponse represents the result from contacting one replica.
type ReplicaResponse struct {
	NodeID string
	Value  *store.Value
	Err    error
}

// Replicator fans reads and writes to replica nodes.
type Replicator struct {
	selfID     string
	membership *Membership
	store      *store.Store
	httpClient *http.Client

	// Quorum parameters
	N int // total replicas per key
	W int // write quorum
	R int // read quorum
}

// NewReplicator creates a new replicator.
//
// IMPORTANT:
// For strong consistency, caller must ensure:
//
//	W + R > N
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

////////////////////////////////////////////////////////////////////////////////
// WRITE PATH
////////////////////////////////////////////////////////////////////////////////

// ReplicateWrite performs a quorum write.
//
// Steps:
//
// 1) Write locally first (coordinator always participates).
// 2) Find replica nodes using consistent hashing.
// 3) Send write to peers in parallel.
// 4) Wait until W acknowledgements are received.
// 5) If quorum reached → success.
// 6) If timeout or insufficient acks → failure.
//
// Self always counts as 1 acknowledgement.
func (rep *Replicator) ReplicateWrite(key, data string, clock store.VectorClock) (store.Value, error) {

	// Step 1: Write locally.
	val, err := rep.store.Put(key, data, clock)
	if err != nil {
		return store.Value{}, fmt.Errorf("local write: %w", err)
	}

	// Step 2: Determine replicas.
	replicas := rep.membership.ReplicaNodes(key, rep.N)
	peers := rep.peersOnly(replicas) // exclude self

	type result struct {
		nodeID string
		err    error
	}
	results := make(chan result, len(peers))

	// Step 3: Send writes in parallel.
	for _, peer := range peers {
		go func(p *Node) {
			err := rep.sendReplicateRequest(p, key, val)
			results <- result{p.ID, err}
		}(peer)
	}

	// Step 4: Wait for quorum.
	acks := 1 // self already acknowledged
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

////////////////////////////////////////////////////////////////////////////////
// READ PATH
////////////////////////////////////////////////////////////////////////////////

// CoordinateRead performs a quorum read.
//
// Steps:
//
// 1) Identify N replica nodes.
// 2) Ask all replicas in parallel.
// 3) Wait for R responses.
// 4) Compare versions using vector clocks.
// 5) Return the newest value.
// 6) If stale replicas detected → trigger read repair.
//
// Read repair keeps replicas eventually consistent.
func (rep *Replicator) CoordinateRead(key string) (*store.Value, error) {

	replicas := rep.membership.ReplicaNodes(key, rep.N)
	responses := make(chan ReplicaResponse, len(replicas))

	// Step 1 & 2: Query replicas in parallel.
	for _, node := range replicas {
		go func(n *Node) {
			if n.ID == rep.selfID {
				// Local read.
				v, ok := rep.store.GetRaw(key)
				if !ok {
					responses <- ReplicaResponse{NodeID: n.ID, Value: nil}
					return
				}
				responses <- ReplicaResponse{NodeID: n.ID, Value: &v}
			} else {
				// Remote read.
				v, err := rep.fetchFromPeer(n, key)
				responses <- ReplicaResponse{NodeID: n.ID, Value: v, Err: err}
			}
		}(node)
	}

	// Step 3: Wait for R responses.
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

	// Step 4: Reconcile versions.
	winner, stale := reconcile(collected)

	if winner == nil {
		return nil, nil // not found
	}
	if winner.Tombstone {
		return nil, nil // deleted
	}

	// Step 6: Repair stale replicas asynchronously.
	if len(stale) > 0 {
		go rep.readRepair(key, *winner, stale)
	}

	return winner, nil
}

////////////////////////////////////////////////////////////////////////////////
// VERSION RECONCILIATION
////////////////////////////////////////////////////////////////////////////////

// reconcile selects the newest value using vector clocks.
//
// Vector clock comparison:
//
//	After     → strictly newer
//	Before    → strictly older
//	Concurrent→ conflict
//
// If concurrent, we use wall-clock time as a tiebreaker.
//
// Returns:
//   - The winning value
//   - List of stale node IDs
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
			staleNodes = append(staleNodes, "")
			winner = r.Value
		case store.Before:
			staleNodes = append(staleNodes, r.NodeID)
		case store.ConcurrentClocks:
			if r.Value.UpdatedAt.After(winner.UpdatedAt) {
				winner = r.Value
			} else {
				staleNodes = append(staleNodes, r.NodeID)
			}
		}
	}
	return winner, staleNodes
}

// readRepair fixes stale replicas.
//
// Instead of running a background anti-entropy job,
// we repair during reads.
//
// This keeps replicas synchronized naturally.
func (rep *Replicator) readRepair(key string, val store.Value, staleNodeIDs []string) {
	for _, id := range staleNodeIDs {
		node, ok := rep.membership.GetNode(id)
		if !ok {
			continue
		}
		_ = rep.sendReplicateRequest(node, key, val) // best effort
	}
}

////////////////////////////////////////////////////////////////////////////////
// HTTP TRANSPORT
////////////////////////////////////////////////////////////////////////////////

// ReplicateRequest is the JSON message sent between nodes.
type ReplicateRequest struct {
	Key   string      `json:"key"`
	Value store.Value `json:"value"`
}

// sendReplicateRequest sends data to a peer.
//
// It uses exponential backoff:
//
//	Attempt 1 → immediate
//	Attempt 2 → wait 100ms
//	Attempt 3 → wait 200ms
//	Attempt 4 → wait 400ms
//
// Why?
// If a node is overloaded,
// retrying instantly makes things worse.
// Backoff reduces pressure.
func (rep *Replicator) sendReplicateRequest(peer *Node, key string, val store.Value) error {

	body := ReplicateRequest{Key: key, Value: val}

	const maxRetries = 3

	for attempt := range maxRetries {

		if attempt > 0 {
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

// doHTTPReplicate performs the actual HTTP POST.
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

// fetchFromPeer retrieves a value from another node.
//
// We fetch raw values including tombstones
// so reconciliation logic can decide correctly.
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

// peersOnly removes self from replica list.
//
// The coordinator already executed locally.
func (rep *Replicator) peersOnly(nodes []*Node) []*Node {
	var peers []*Node
	for _, n := range nodes {
		if n.ID != rep.selfID {
			peers = append(peers, n)
		}
	}
	return peers
}

// DeleteReplicated performs a quorum delete.
//
// Deletes are implemented using tombstones.
// This prevents deleted data from reappearing
// during reconciliation.
func (rep *Replicator) DeleteReplicated(key string) error {

	// Local delete first.
	if err := rep.store.Delete(key); err != nil {
		return err
	}

	// Fetch tombstone value.
	val, _ := rep.store.GetRaw(key)

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
