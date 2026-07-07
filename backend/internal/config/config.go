package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port                string
	JWTSecret           string
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	LogLevel            string
	AllowAllCORS        bool
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	ShutdownGrace       time.Duration
	MongoEnabled        bool
	MongoURI            string
	MongoDatabase       string
	MongoTimeout        time.Duration
	ClawSynapseAPIURL   string
	ClawSynapseTimeout  time.Duration
	ClawSynapsePeerSync time.Duration

	// Knowledge base
	EmbeddingProvider  string
	EmbeddingModel     string
	EmbeddingAPIKey    string
	EmbeddingAPIURL    string
	EmbeddingDimension int
	QdrantURL          string
	KnowledgeStorePath string

	// Assistant (LLM-powered)
	AssistantAPIURL string
	AssistantAPIKey string
	AssistantModel  string

	// Project files
	FilesStoragePath string

	// External URL exposed to agents for file download (e.g. "http://192.168.1.100:8080")
	ExternalURL string

	// Download token TTL for agent file access
	DownloadTokenTTL time.Duration

	// Market
	MarketDataPath string

	// Platform branding
	PlatformName string
}

func Load() Config {
	return Config{
		Port:                getEnv("PORT", "8080"),
		JWTSecret:           getEnv("JWT_SECRET", "trustmesh-dev-secret"),
		AccessTokenTTL:      getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:     getEnvDuration("REFRESH_TOKEN_TTL", 168*time.Hour),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		AllowAllCORS:        getEnvBool("ALLOW_ALL_CORS", true),
		ReadTimeout:         getEnvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:        getEnvDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:         getEnvDuration("IDLE_TIMEOUT", 120*time.Second),
		ShutdownGrace:       getEnvDuration("SHUTDOWN_GRACE", 8*time.Second),
		MongoEnabled:        getEnvBool("MONGO_ENABLED", true),
		MongoURI:            getEnv("MONGO_URI", "mongodb://127.0.0.1:27017"),
		MongoDatabase:       getEnv("MONGO_DATABASE", "trustmesh"),
		MongoTimeout:        getEnvDuration("MONGO_TIMEOUT", 5*time.Second),
		ClawSynapseAPIURL:   getEnv("CLAWSYNAPSE_API_URL", "http://127.0.0.1:18080"),
		ClawSynapseTimeout:  getEnvDuration("CLAWSYNAPSE_TIMEOUT", 3*time.Second),
		ClawSynapsePeerSync: getEnvDuration("CLAWSYNAPSE_PEER_SYNC_INTERVAL", 10*time.Second),

		EmbeddingProvider:  getEnv("EMBEDDING_PROVIDER", "openai"),
		EmbeddingModel:     getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),
		EmbeddingAPIKey:    getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingAPIURL:    getEnv("EMBEDDING_API_URL", "https://api.openai.com/v1"),
		EmbeddingDimension: getEnvInt("EMBEDDING_DIMENSION", 1536),
		QdrantURL:          getEnv("QDRANT_URL", "http://127.0.0.1:6333"),
		KnowledgeStorePath: getEnv("KNOWLEDGE_STORAGE_PATH", "/var/lib/trustmesh-knowledge"),

		FilesStoragePath: getEnv("FILES_STORAGE_PATH", "/var/lib/trustmesh-files"),

		ExternalURL:      getEnv("TRUSTMESH_EXTERNAL_URL", "http://127.0.0.1:8080"),
		DownloadTokenTTL: getEnvDuration("DOWNLOAD_TOKEN_TTL", 10*time.Minute),

		AssistantAPIURL: getEnv("ASSISTANT_API_URL", "https://api.openai.com/v1"),
		AssistantAPIKey: getEnv("ASSISTANT_API_KEY", ""),
		AssistantModel:  getEnv("ASSISTANT_MODEL", "gpt-4o-mini"),

		MarketDataPath: getEnv("MARKET_DATA_PATH", "data/roles_index.json"),

		PlatformName: getEnv("PLATFORM_NAME", "TrustMesh"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return fallback
}
