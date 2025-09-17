package api

import (
	"distributed-kvstore/internal/cluster"
	"distributed-kvstore/internal/store"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type API struct {
	node  *cluster.Node
	store *store.Store
}

func NewAPI(node *cluster.Node, store *store.Store) *API {
	return &API{
		node:  node,
		store: store,
	}
}

func (a *API) SetupRoutes(r *gin.Engine) {
	// public API endpoints for client access
	kv := r.Group("/kv")
	{
		kv.PUT("/:key", a.PutKey)
		kv.GET("/:key", a.GetKey)
		kv.DELETE("/:key", a.DeleteKey)
	}

	// internal API endpoints for inter-node communication and replication
	internal := r.Group("/internal")
	{
		internal.PUT("/replicate", a.ReplicatePut)
		internal.GET("/replicate/:key", a.ReplicateGet)
		internal.DELETE("/replicate/:key", a.ReplicateDelete)
	}

	// cluster management endpoints
	cluster := r.Group("/cluster")
	{
		cluster.POST("/join", a.JoinCluster)
		cluster.GET("/status", a.ClusterStatus)
		cluster.POST("/leave", a.LeaveCluster)
	}

	// adming and monitoring endpoints
	admin := r.Group("/admin")
	{
		admin.GET("/shards", a.GetShards)
		admin.GET("/replication", a.GetReplicationStatus)
		admin.POST("/snapshot", a.CreateSnapshot)
	}
}

// the req body format for the PUT /kv/:key endpoing
type PUTrequest struct {
	Value string `json:"value" binding:"required"`
}

func (a *API) PutKey(c *gin.Context) {
	key := c.Param("key")
	var req PUTrequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// calls the node's PUT method to handle the distributed write operation
	if err := a.node.Put(key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (a *API) GetKey(c *gin.Context) {
	key := c.Param("key")

	// calls the node's GET method to hander dist. read op
	entry, err := a.node.Get(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// checks if the key was found or marked as deleted
	if entry == nil || entry.Deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"value":   entry.Value,
		"version": entry.Version,
	})
}

func (a *API) DeleteKey(c *gin.Context) {
	key := c.Param("key")

	if err := a.node.Delete(key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// internal endpoint for a node to replicate a write operation
func (a *API) ReplicatePut(c *gin.Context) {
	var req cluster.QuorumRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// directly calls the local store's PUT method
	err := a.store.Put(req.Key, req.Value, req.Version)
	success := err == nil

	c.JSON(http.StatusOK, cluster.QuorumResponse{
		Success: success,
		Error: func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(),
	})
}

// internal endpoint for a node to replicate a read op
func (a *API) ReplicateGet(c *gin.Context) {
	key := c.Param("key")

	entry, exists := a.store.Get(key)
	if !exists {
		// if the key doesn't exist, responds with a failed QuorumResponse
		c.JSON(http.StatusOK, cluster.QuorumResponse{Success: false})
		return
	}

	c.JSON(http.StatusOK, cluster.QuorumResponse{
		Success: true,
		Value:   entry.Value,
		Version: &entry.Version,
	})
}

// internal endpoint for a node to replicate a delete op
func (a *API) ReplicateDelete(c *gin.Context) {
	key := c.Param("key")

	err := a.store.Delete(key)
	success := err == nil

	c.JSON(http.StatusOK, cluster.QuorumResponse{
		Success: success,
		Error: func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(),
	})
}

// cluster management handlers

// handles a req from a new node to join the cluster
// NOTE: the implementation here is a placeholder.
// real world logic would involve adding the new node to the consistent hash ring
// and potentially rebalancing data
func (a *API) JoinCluster(c *gin.Context) {
	var req struct {
		NodeID  string `json:"node_id"`
		Address string `json:"address"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "joined"})
}

// provides a health check and metrics for the node.
// NOTE: The node ID is hardcoded here and should be retrieved from the node's config.
func (a *API) ClusterStatus(c *gin.Context) {
	metrics := a.store.GetMetrics()

	status := map[string]any{
		"node_id":   "node-1", // placeholder
		"status":    "healthy",
		"reads":     metrics.Reads,
		"writes":    metrics.Writes,
		"deletes":   metrics.Deletes,
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, status)
}

// handles a req for a node to leave
// NOTE: again a placeholder
// actual logic would involve removing teh node from the consistent hash
// and migrating its data to other nodes
func (a *API) LeaveCluster(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "left"})
}

// Admin handlers

// provides information about the data distribution.
// NOTE: This is a placeholder and would require logic to show which keys map to which nodes.
func (a *API) GetShards(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"shards": "implementation needed"})
}

// provides information about the replication state.
// NOTE: This is a placeholder and would require logic to track replication progress
func (a *API) GetReplicationStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"replication": "implementation needed"})
}

// handles an admin req to create a snapshot of the local store
func (a *API) CreateSnapshot(c *gin.Context) {
	if err := a.store.Snapshot(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "snapshot created"})
}
