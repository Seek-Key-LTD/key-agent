// oracle-agent demonstrates importing Oracle-backed memory and session backends.
package main

import (
	"flag"
	"log"
	"os"

	memOracle "google.golang.org/adk/v2/memory/oracle"
	"google.golang.org/adk/v2/model/openaimodel"
	sessOracle "google.golang.org/adk/v2/session/oracle"

	_ "google.golang.org/adk/v2/memory/classifier"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("ORACLE_DSN"), "Oracle DSN")
	agentName := flag.String("agent", "oracle-agent", "Agent name")
	modelID := flag.String("model", "nova-deepseek-v4-flash", "Model ID")
	baseURL := flag.String("base-url", os.Getenv("OPENAI_BASE_URL"), "LLM base URL")
	secret := flag.String("secret", os.Getenv("LITELLM_API_KEY"), "LiteLLM API key")
	memoryKey := flag.String("memory-key", os.Getenv("ORACLE_MEMORY_KEY"), "AES-256 key hex")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("-dsn required (or set ORACLE_DSN)")
	}
	if *baseURL == "" {
		log.Fatal("-base-url required (or set OPENAI_BASE_URL)")
	}

	llm, err := openaimodel.NewModel(nil, *modelID, &openaimodel.ClientConfig{
		BaseURL: *baseURL,
		APIKey:  *secret,
	})
	if err != nil {
		log.Fatalf("create LLM: %v", err)
	}

	mem := memOracle.NewOracleMemory(*dsn, *memoryKey)
	sess := sessOracle.NewOracleSession(*dsn)

	_ = llm
	_ = mem
	_ = sess

	log.Printf("PicoOracle Agent %q | LLM: %s@%s | Oracle: %s",
		*agentName, *modelID, *baseURL, maskDSN(*dsn))
	log.Printf("Memory backend: Oracle (3-layer, AES-256-GCM)")
	log.Printf("Session backend: Oracle (PICO_SESSION)")
	log.Printf("Ready to connect: dsn=%s", maskDSN(*dsn))
}

func maskDSN(dsn string) string { return "***" }
