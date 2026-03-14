package config

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client
var Ctx = context.Background()

func ConnectRedis() {
	redisURL := os.Getenv("REDIS_URL")
	redisPrivURL := os.Getenv("REDIS_PRIVATE_URL")
	redisPubURL := os.Getenv("REDIS_PUBLIC_URL")
	redisHost := os.Getenv("REDISHOST")
	redisPort := os.Getenv("REDISPORT")
	redisUser := os.Getenv("REDISUSER")
	redisPass := os.Getenv("REDISPASSWORD")
	oldRedisHost := os.Getenv("REDIS_HOST")
	oldRedisPort := os.Getenv("REDIS_PORT")
	oldRedisPass := os.Getenv("REDIS_PASSWORD")

	fmt.Println("--- Redis Environment Variable Check ---")
	fmt.Printf("REDIS_URL: %t, REDIS_PRIVATE_URL: %t, REDIS_PUBLIC_URL: %t\n", redisURL != "", redisPrivURL != "", redisPubURL != "")
	fmt.Printf("REDISHOST: %s, REDISPORT: %s, REDISUSER: %s\n", redisHost, redisPort, redisUser)
	fmt.Printf("REDIS_HOST: %s, REDIS_PORT: %s\n", oldRedisHost, oldRedisPort)
	fmt.Println("---------------------------------------")

	var options *redis.Options

	if redisURL != "" || redisPrivURL != "" || redisPubURL != "" {
		urlToUse := redisURL
		if urlToUse == "" { urlToUse = redisPrivURL }
		if urlToUse == "" { urlToUse = redisPubURL }

		opt, err := redis.ParseURL(urlToUse)
		if err != nil {
			log.Fatalf("Failed to parse Redis URL: %v", err)
		}
		options = opt
		log.Println("Using Redis URL for connection")
	} else {
		host := redisHost
		if host == "" { host = oldRedisHost }
		if host == "" { host = "localhost" }

		port := redisPort
		if port == "" { port = oldRedisPort }
		if port == "" { port = "6379" }

		pass := redisPass
		if pass == "" { pass = oldRedisPass }

		addr := fmt.Sprintf("%s:%s", host, port)
		options = &redis.Options{
			Addr:     addr,
			Username: redisUser,
			Password: pass,
			DB:       0,
		}
		log.Printf("Using constructed Redis Addr: %s", addr)
	}

	RedisClient = redis.NewClient(options)

	_, err := RedisClient.Ping(Ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v. Addr attempted: %s", err, options.Addr)
	}

	log.Println("Redis connection successfully opened")
}
