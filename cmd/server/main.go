// cmd/server is the main entrypoint for a KV store node.
//
// Configuration is entirely via flags/environment so a single binary can
// serve any role in the cluster.
//
// Example — single node:
//
//	./server --id node1 --addr :8080 --data-dir /var/kvstore/node1
//
// Example — 3-node cluster:
//
//	./server --id node1 --addr :8080 --data-dir /tmp/n1 \
//	         --peers node2=localhost:8081,node3=localhost:8082
//	./server --id node2 --addr :8081 --data-dir /tmp/n2 \
//	         --peers node1=localhost:8080,node3=localhost:8082
//	./server --id node3 --addr :8082 --data-dir /tmp/n3 \
//	         --peers node1=localhost:8080,node2=localhost:8081
package main

import (
	"context"
	"distributed-kvstore/internal/api"
	"distributed-kvstore/internal/cluster"
	"distributed-kvstore/internal/store"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// ── Flags ──────────────────────────────────────────────────────────────
	nodeID := flag.String("id", "node1", "Unique node identifier")
	addr := flag.String("addr", ":8080", "Listen address (host:port)")
	dataDir := flag.String("data-dir", "/tmp/kvstore", "Directory for WAL and snapshots")
	peersFlag := flag.String("peers", "", "Comma-separated list of peer nodes: id=host:port")
	replicationN := flag.Int("n", 3, "Replication factor (N)")
	writeQuorum := flag.Int("w", 2, "Write quorum (W)")
	readQuorum := flag.Int("r", 2, "Read quorum (R)")
	flag.Parse()

	if *writeQuorum+*readQuorum <= *replicationN {
		log.Fatalf("FATAL: W(%d) + R(%d) must be > N(%d) for strong consistency",
			*writeQuorum, *readQuorum, *replicationN)
	}

	// ── Storage ────────────────────────────────────────────────────────────
	nodeDataDir := fmt.Sprintf("%s/%s", *dataDir, *nodeID)
	s, err := store.New(nodeDataDir, *nodeID)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()

	// ── Cluster membership ─────────────────────────────────────────────────
	// Always add self to the membership list.
	selfNode := cluster.Node{ID: *nodeID, Address: *addr}
	nodes := []cluster.Node{selfNode}

	if *peersFlag != "" {
		for _, entry := range strings.Split(*peersFlag, ",") {
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) != 2 {
				log.Fatalf("invalid peer format %q: expected id=host:port", entry)
			}
			nodes = append(nodes, cluster.Node{ID: parts[0], Address: parts[1]})
		}
	}

	membership := cluster.NewMembership(nodes, 150)

	// ── Replicator ─────────────────────────────────────────────────────────
	// If there are fewer nodes than N, cap quorum to avoid deadlock.
	n := min(*replicationN, membership.Ring().NodeCount())
	w := min(*writeQuorum, n)
	r := min(*readQuorum, n)
	replicator := cluster.NewReplicator(*nodeID, membership, s, n, w, r)

	// ── HTTP server ────────────────────────────────────────────────────────
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(api.Logger(), api.Recovery())

	handler := api.NewHandler(s, replicator, membership, *nodeID)
	handler.Register(router)

	// Health check endpoint — useful for load balancers and readiness probes.
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"node":   *nodeID,
			"status": "ok",
			"nodes":  membership.Ring().NodeCount(),
		})
	})

	srv := &http.Server{
		Addr:         *addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// ── Graceful shutdown ──────────────────────────────────────────────────
	// Listen for SIGINT/SIGTERM and give in-flight requests 15s to complete.
	go func() {
		log.Printf("Node %s listening on %s (N=%d W=%d R=%d)", *nodeID, *addr, n, w, r)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Background snapshot every 60 seconds.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.Snapshot(); err != nil {
				log.Printf("snapshot error: %v", err)
			} else {
				log.Printf("snapshot saved")
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down node", *nodeID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Take a final snapshot before exiting.
	if err := s.Snapshot(); err != nil {
		log.Printf("final snapshot error: %v", err)
	}

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
