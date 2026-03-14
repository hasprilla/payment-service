package utils

import (
	"context"
	"log"
	"os"

	"github.com/segmentio/kafka-go"
)

var kafkaWriter *kafka.Writer

func InitKafka() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}

	kafkaWriter = &kafka.Writer{
		Addr:     kafka.TCP(brokers),
		Topic:    "wallet-events",
		Balancer: &kafka.LeastBytes{},
	}
	log.Println("Kafka Producer initialized for topic 'wallet-events'")
}

func PublishEvent(ctx context.Context, key string, value []byte) error {
	err := kafkaWriter.WriteMessages(ctx,
		kafka.Message{
			Key:   []byte(key),
			Value: value,
		},
	)
	if err != nil {
		log.Printf("Failed to publish message to Kafka: %v", err)
		return err
	}
	return nil
}

func CloseKafka() {
	if kafkaWriter != nil {
		kafkaWriter.Close()
	}
}
