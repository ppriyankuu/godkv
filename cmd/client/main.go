// cmd/client is the CLI entry-point built with Cobra.
//
// Usage:
//
//	kvcli put mykey "hello world"      --server http://localhost:8080
//	kvcli get mykey                    --server http://localhost:8080
//	kvcli delete mykey                 --server http://localhost:8080
//	kvcli cluster nodes                --server http://localhost:8080
package main

import (
	"context"
	"distributed-kvstore/internal/client"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	timeout    time.Duration
)

func main() {
	root := &cobra.Command{
		Use:   "kvcli",
		Short: "CLI client for the distributed KV store",
	}

	root.PersistentFlags().StringVarP(&serverAddr, "server", "s",
		"http://localhost:8080", "KV store server address")
	root.PersistentFlags().DurationVar(&timeout, "timeout", 10*time.Second,
		"HTTP request timeout")

	root.AddCommand(putCmd(), getCmd(), deleteCmd(), clusterCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ─── put ──────────────────────────────────────────────────────────────────────

func putCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "put <key> <value>",
		Short: "Store a key-value pair",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New(serverAddr, timeout)
			resp, err := c.Put(context.Background(), args[0], args[1])
			if err != nil {
				return err
			}
			prettyPrint(resp)
			return nil
		},
	}
}

// ─── get ──────────────────────────────────────────────────────────────────────

func getCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Retrieve a value by key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New(serverAddr, timeout)
			resp, err := c.Get(context.Background(), args[0])
			if err == client.ErrNotFound {
				fmt.Printf("key %q not found\n", args[0])
				return nil
			}
			if err != nil {
				return err
			}
			prettyPrint(resp)
			return nil
		},
	}
}

// ─── delete ───────────────────────────────────────────────────────────────────

func deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <key>",
		Short: "Delete a key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New(serverAddr, timeout)
			if err := c.Delete(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("deleted %q\n", args[0])
			return nil
		},
	}
}

// ─── cluster ──────────────────────────────────────────────────────────────────

func clusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Cluster management commands",
	}

	// cluster nodes
	cmd.AddCommand(&cobra.Command{
		Use:   "nodes",
		Short: "List all cluster nodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New(serverAddr, timeout)
			ctx := context.Background()
			// Simple GET to /cluster/nodes
			resp, err := c.GetRaw(ctx, "/cluster/nodes")
			if err != nil {
				return err
			}
			fmt.Println(resp)
			return nil
		},
	})

	// cluster join
	joinCmd := &cobra.Command{
		Use:   "join <nodeID> <address>",
		Short: "Join a node to the cluster",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New(serverAddr, timeout)
			return c.JoinCluster(context.Background(), args[0], args[1])
		},
	}

	// cluster leave
	leaveCmd := &cobra.Command{
		Use:   "leave <nodeID>",
		Short: "Remove a node from the cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New(serverAddr, timeout)
			return c.LeaveCluster(context.Background(), args[0])
		},
	}

	cmd.AddCommand(joinCmd, leaveCmd)
	return cmd
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func prettyPrint(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(data))
}
