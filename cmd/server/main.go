package main

import (
	"distributed-kvstore/internal/api"
	"distributed-kvstore/internal/cluster"
	"distributed-kvstore/internal/store"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// validate quorum settings
	if config.ReadQuorum+config.WriteQuorum <= config.Replication {
		log.Fatalf("Invalid quorum settings: R+W must be > N")
	}

	// store initialization
	walPath := fmt.Sprintf("node_%s.wal", config.NodeID)
	kvstore, err := store.NewStore(config.NodeID, walPath)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}

	// cluster node initialization
	node := cluster.NewNode(config, kvstore)

	r := gin.Default()

	apiHandler := api.NewAPI(node, kvstore)
	apiHandler.SetupRoutes(r)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if err := kvstore.Snapshot(); err != nil {
				log.Printf("failed to create snapshot: %v", err)
			}
		}
	}()

	log.Printf("starting kv store on %s", config.Address)
	log.Fatal(r.Run(config.Address))
}

func loadConfig(path string) (*cluster.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config cluster.Config

	err = json.Unmarshal(data, &config)
	return &config, err
}
