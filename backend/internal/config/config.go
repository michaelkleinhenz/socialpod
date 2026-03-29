package config

import "os"

type Config struct {
	MongoURI    string
	MongoDBName string
	JWTSecret   string
	AppURL      string
	APIPort     string
	UploadDir   string
}

func Load() *Config {
	return &Config{
		MongoURI:    getEnv("MONGO_URI", "mongodb://socialmedia:socialmedia_secret@localhost:27017/?authSource=admin"),
		MongoDBName: getEnv("MONGO_DATABASE", "socialmedia"),
		JWTSecret:   getEnv("JWT_SECRET", "change-me-in-production"),
		AppURL:      getEnv("APP_URL", "http://localhost:3000"),
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
