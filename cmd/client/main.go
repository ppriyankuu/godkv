package main

import (
	"fmt"
	"log"
	"os"

	"distributed-kvstore/internal/client"

	"github.com/spf13/cobra"
)

var (
	serverURL string
	// kvClient  *client.Client
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "kvstore-cli",
		Short: "KV Store CLI client",
	}

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8080", "KV store server URL")

	var putCmd = &cobra.Command{
		Use:   "put [key] [value]",
		Short: "Put a key-value pair",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			client := client.NewClient(serverURL)
			if err := client.Put(args[0], args[1]); err != nil {
				log.Fatalf("Failed to put: %v", err)
			}
			fmt.Println("OK")
		},
	}

	var getCmd = &cobra.Command{
		Use:   "get [key]",
		Short: "Get a value by key",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := client.NewClient(serverURL)
			value, err := client.Get(args[0])
			if err != nil {
				log.Fatalf("Failed to get: %v", err)
			}
			fmt.Println(value)
		},
	}

	var delCmd = &cobra.Command{
		Use:   "delete [key]",
		Short: "Delete a key",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := client.NewClient(serverURL)
			if err := client.Delete(args[0]); err != nil {
				log.Fatalf("Failed to delete: %v", err)
			}
			fmt.Println("Deleted")
		},
	}

	rootCmd.AddCommand(putCmd, getCmd, delCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
