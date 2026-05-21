package nodes

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Kafka produces or consumes messages from Apache Kafka.
// Actions: produce, consume.
//
// Example producer:
//
//	nodes.NewKafka([]string{"localhost:9092"}, "my-topic")
type Kafka struct {
	Brokers []string
	Topic   string
	GroupID string // consumer group ID for consume action
}

func NewKafka(brokers []string, topic string) *Kafka {
	return &Kafka{Brokers: brokers, Topic: topic}
}

func (k *Kafka) WithGroupID(id string) *Kafka {
	k.GroupID = id
	return k
}

func (k *Kafka) Name() string { return "kafka" }

func (k *Kafka) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Produces or consumes messages from Apache Kafka.",
		Params: map[string]any{
			"action":   map[string]any{"type": "string", "desc": "produce | consume"},
			"topic":    map[string]any{"type": "string", "desc": "Kafka topic. Falls back to constructor."},
			"message":  map[string]any{"type": "string", "desc": "Message value to produce."},
			"key":      map[string]any{"type": "string", "desc": "Message key (produce)."},
			"limit":    map[string]any{"type": "number", "desc": "Max messages to consume (consume). Default 1."},
			"group_id": map[string]any{"type": "string", "desc": "Consumer group ID (consume)."},
		},
	}
}

func (k *Kafka) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	if action == "" {
		action = "produce"
	}

	topic := k.Topic
	if v, ok := in["topic"].(string); ok && v != "" {
		topic = v
	}
	if topic == "" {
		return nil, fmt.Errorf("kafka: topic is required")
	}

	switch action {
	case "produce":
		msg, _ := in["message"].(string)
		if msg == "" {
			return nil, fmt.Errorf("kafka: message is required for produce")
		}
		key, _ := in["key"].(string)

		w := kafka.NewWriter(kafka.WriterConfig{
			Brokers:      k.Brokers,
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			WriteTimeout: 10 * time.Second,
		})
		defer w.Close()

		km := kafka.Message{Value: []byte(msg)}
		if key != "" {
			km.Key = []byte(key)
		}

		if err := w.WriteMessages(ctx, km); err != nil {
			return nil, fmt.Errorf("kafka: produce: %w", err)
		}
		return orchkit.Output{"sent": true, "topic": topic, "message": msg}, nil

	case "consume":
		limit := 1
		if v, ok := in["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		groupID := k.GroupID
		if v, ok := in["group_id"].(string); ok && v != "" {
			groupID = v
		}

		r := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   k.Brokers,
			Topic:     topic,
			GroupID:   groupID,
			MaxWait:   5 * time.Second,
			MinBytes:  1,
			MaxBytes:  1e6,
		})
		defer r.Close()

		messages := make([]any, 0, limit)
		for i := 0; i < limit; i++ {
			msg, err := r.ReadMessage(ctx)
			if err != nil {
				break
			}
			messages = append(messages, map[string]any{
				"key":       string(msg.Key),
				"value":     string(msg.Value),
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"time":      msg.Time.Format(time.RFC3339),
			})
		}
		return orchkit.Output{"messages": messages, "count": len(messages)}, nil

	default:
		return nil, fmt.Errorf("kafka: unknown action %q", action)
	}
}
