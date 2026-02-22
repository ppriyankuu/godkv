// Package api wires up the Gin HTTP router with all handler functions.
package api

import (
	"distributed-kvstore/internal/cluster"
	"distributed-kvstore/internal/store"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler holds all dependencies injected from main.
type Handler struct {
	store      *store.Store
	replicator *cluster.Replicator
	membership *cluster.Membership
	selfID     string
}

// NewHandler creates a Handler.
func NewHandler(s *store.Store, r *cluster.Replicator, m *cluster.Membership, selfID string) *Handler {
	return &Handler{store: s, replicator: r, membership: m, selfID: selfID}
}

// Register mounts all routes on r.
func (h *Handler) Register(r *gin.Engine) {
	// Public KV API — used by clients.
	kv := r.Group("/kv")
	kv.GET("/:key", h.Get)
	kv.PUT("/:key", h.Put)
	kv.DELETE("/:key", h.Delete)

	// Cluster management.
	clusterGroup := r.Group("/cluster")
	clusterGroup.POST("/join", h.Join)
	clusterGroup.POST("/leave", h.Leave)
	clusterGroup.GET("/nodes", h.ListNodes)

	// Internal endpoints used only by peer nodes.
	internal := r.Group("/internal")
	internal.POST("/replicate", h.InternalReplicate)
	internal.GET("/fetch/:key", h.InternalFetch)
}

// ─── Public KV handlers ───────────────────────────────────────────────────────

// Put handles PUT /kv/:key
// Body: {"value": "<string>"}
func (h *Handler) Put(c *gin.Context) {
	key := c.Param("key")

	var body struct {
		Value string `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	val, err := h.replicator.ReplicateWrite(key, body.Value, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":   key,
		"value": val.Data,
		"clock": val.Clock,
	})
}

// Get handles GET /kv/:key
func (h *Handler) Get(c *gin.Context) {
	key := c.Param("key")

	val, err := h.replicator.CoordinateRead(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if val == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":        key,
		"value":      val.Data,
		"clock":      val.Clock,
		"updated_at": val.UpdatedAt,
	})
}

// Delete handles DELETE /kv/:key
func (h *Handler) Delete(c *gin.Context) {
	key := c.Param("key")

	if err := h.replicator.DeleteReplicated(key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": key})
}

// ─── Cluster management handlers ─────────────────────────────────────────────

// Join handles POST /cluster/join
// Body: {"id": "<nodeID>", "address": "<host:port>"}
func (h *Handler) Join(c *gin.Context) {
	var node cluster.Node
	if err := c.ShouldBindJSON(&node); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.membership.Join(node); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"joined": node.ID})
}

// Leave handles POST /cluster/leave
// Body: {"id": "<nodeID>"}
func (h *Handler) Leave(c *gin.Context) {
	var body struct {
		ID string `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.membership.Leave(body.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"left": body.ID})
}

// ListNodes handles GET /cluster/nodes
func (h *Handler) ListNodes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"nodes": h.membership.All()})
}

// ─── Internal (peer-to-peer) handlers ────────────────────────────────────────

// InternalReplicate handles POST /internal/replicate
// Accepts a value from a peer and applies it using vector-clock conflict resolution.
func (h *Handler) InternalReplicate(c *gin.Context) {
	var req cluster.ReplicateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.store.ApplyRemote(req.Key, req.Value)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// InternalFetch handles GET /internal/fetch/:key
// Returns the raw value (including tombstones) so peers can do read repair.
func (h *Handler) InternalFetch(c *gin.Context) {
	key := c.Param("key")
	val, ok := h.store.GetRaw(key)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, val)
}
