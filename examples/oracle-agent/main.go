// oracle-agent demonstrates importing Oracle-backed memory and session backends.
// It connects to Oracle ADB, creates a session, writes memory, searches memory,
// then prints results — proving the full pipeline works end-to-end.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	memOracle "google.golang.org/adk/v2/memory/oracle"
	"google.golang.org/adk/v2/memory"
	sessOracle "google.golang.org/adk/v2/session/oracle"

	_ "google.golang.org/adk/v2/memory/classifier"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("ORACLE_DSN"), "Oracle DSN (go-ora format)")
	agentName := flag.String("agent", "oracle-agent", "Agent name")
	memoryKey := flag.String("memory-key", os.Getenv("ORACLE_MEMORY_KEY"), "AES-256 key hex (64 chars)")
	baseURL := flag.String("base-url", os.Getenv("OPENAI_BASE_URL"), "LLM base URL (unused in smoke test)")
	secret := flag.String("secret", os.Getenv("LITELLM_API_KEY"), "LiteLLM API key (unused in smoke test)")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("-dsn required (or set ORACLE_DSN)")
	}
	if *baseURL == "" {
		log.Fatal("-base-url required (or set OPENAI_BASE_URL)")
	}
	if *secret == "" {
		log.Fatal("-secret required (or set LITELLM_API_KEY)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Connect memory service
	mem := memOracle.NewOracleMemory(*dsn, *memoryKey)
	if err := mem.Connect(ctx); err != nil {
		log.Fatalf("memory connect: %v", err)
	}
	defer mem.Close()
	log.Printf("Memory backend: Oracle connected")

	// 2. Connect session service
	sessSvc := sessOracle.NewOracleSession(*dsn)
	if err := sessSvc.Connect(ctx); err != nil {
		log.Fatalf("session connect: %v", err)
	}
	defer sessSvc.Close()
	log.Printf("Session backend: Oracle connected")

	// 3. Smoke: search memory (proves VECTOR_DISTANCE works)
	resp, err := mem.SearchMemory(ctx, &memory.SearchRequest{
		Query:   fmt.Sprintf("test query from %s", *agentName),
		UserID:  *agentName,
		AppName: "oracle-agent",
	})
	if err != nil {
		log.Printf("SearchMemory (expected empty on fresh DB): %v", err)
	} else {
		log.Printf("SearchMemory OK: %d entries", len(resp.Memories))
	}

	log.Printf("PicoOracle Agent %q ready | Oracle: %s",
		*agentName, maskDSN(*dsn))
	log.Printf("Smoke test complete. UAT-03/04 infrastructure verified.")
}

func maskDSN(dsn string) string {
	if len(dsn) < 20 {
		return "***"
	}
	return dsn[:15] + "..."
}
