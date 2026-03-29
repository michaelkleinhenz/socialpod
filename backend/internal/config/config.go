package config

import (
	"fmt"
	"os"
)

type Config struct {
	MongoURI    string
	MongoDBName string
	JWTSecret   string
	AppURL      string
	APIPort     string
	UploadDir   string
}

func Load() *Config {
	mongoURI := getEnv("MONGO_URI", "")
	if mongoURI == "" {
		host := getEnv("MONGO_HOST", "localhost:27017")
		user := getEnv("MONGO_USER", "socialmedia")
		pass := getEnv("MONGO_PASSWORD", "socialmedia_secret")
		mongoURI = fmt.Sprintf("mongodb://%s:%s@%s/?authSource=admin", user, pass, host)
	}

	return &Config{
		MongoURI:    mongoURI,
		MongoDBName: getEnv("MONGO_DATABASE", "socialmedia"),
		JWTSecret:   getEnv("JWT_SECRET", "change-me-in-production"),
		AppURL:      getEnv("APP_URL", "http://localhost:8080"),
		APIPort:     getEnv("API_PORT", "8080"),
		UploadDir:   getEnv("UPLOAD_DIR", "./uploads"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
