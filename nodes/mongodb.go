package nodes

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"github.com/shaiksadikjanu-cmd/orchkit"
)

// MongoDB executes operations against MongoDB.
// Actions: find, find_one, insert_one, insert_many, update_one, delete_one, count.
//
// Example:
//
//	nodes.NewMongoDB("mongodb://localhost:27017", "mydb", "users")
type MongoDB struct {
	URI        string
	Database   string
	Collection string
}

func NewMongoDB(uri, database, collection string) *MongoDB {
	return &MongoDB{URI: uri, Database: database, Collection: collection}
}

func (m *MongoDB) Name() string { return "mongodb" }

func (m *MongoDB) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Executes MongoDB operations: find, find_one, insert_one, update_one, delete_one, count.",
		Params: map[string]any{
			"action":     map[string]any{"type": "string", "desc": "find | find_one | insert_one | insert_many | update_one | delete_one | count"},
			"filter":     map[string]any{"type": "object", "desc": "MongoDB filter document."},
			"document":   map[string]any{"type": "object", "desc": "Document to insert or update body."},
			"update":     map[string]any{"type": "object", "desc": "Update operators e.g. {$set: {field: value}}."},
			"limit":      map[string]any{"type": "integer", "desc": "Max documents to return (find). Default 100."},
			"collection": map[string]any{"type": "string", "desc": "Collection name. Falls back to constructor."},
		},
	}
}

func (m *MongoDB) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	uri := m.URI
	if uri == "" {
		return nil, fmt.Errorf("mongodb: URI is required")
	}

	collName := m.Collection
	if v, ok := in["collection"].(string); ok && v != "" {
		collName = v
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongodb: connect: %w", err)
	}
	defer client.Disconnect(ctx)

	coll := client.Database(m.Database).Collection(collName)

	action, _ := in["action"].(string)
	if action == "" {
		action = "find"
	}

	// Build filter from input.
	filter := bson.D{}
	if f, ok := in["filter"].(map[string]any); ok {
		for k, v := range f {
			filter = append(filter, bson.E{Key: k, Value: v})
		}
	}

	switch action {
	case "find":
		limit := int64(100)
		if v, ok := in["limit"].(float64); ok && v > 0 {
			limit = int64(v)
		}
		cursor, err := coll.Find(ctx, filter, options.Find().SetLimit(limit))
		if err != nil {
			return nil, fmt.Errorf("mongodb: find: %w", err)
		}
		defer cursor.Close(ctx)
		var docs []any
		if err := cursor.All(ctx, &docs); err != nil {
			return nil, fmt.Errorf("mongodb: decode: %w", err)
		}
		if docs == nil {
			docs = []any{}
		}
		return orchkit.Output{"docs": docs, "count": len(docs)}, nil

	case "find_one":
		var doc map[string]any
		err := coll.FindOne(ctx, filter).Decode(&doc)
		if err == mongo.ErrNoDocuments {
			return orchkit.Output{"doc": nil, "found": false}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("mongodb: find_one: %w", err)
		}
		return orchkit.Output{"doc": doc, "found": true}, nil

	case "insert_one":
		doc, _ := in["document"].(map[string]any)
		if doc == nil {
			return nil, fmt.Errorf("mongodb: document required for insert_one")
		}
		result, err := coll.InsertOne(ctx, doc)
		if err != nil {
			return nil, fmt.Errorf("mongodb: insert_one: %w", err)
		}
		return orchkit.Output{"inserted_id": fmt.Sprint(result.InsertedID)}, nil

	case "update_one":
		update, _ := in["update"].(map[string]any)
		if update == nil {
			return nil, fmt.Errorf("mongodb: update required for update_one")
		}
		result, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			return nil, fmt.Errorf("mongodb: update_one: %w", err)
		}
		return orchkit.Output{
			"matched_count":  result.MatchedCount,
			"modified_count": result.ModifiedCount,
		}, nil

	case "delete_one":
		result, err := coll.DeleteOne(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("mongodb: delete_one: %w", err)
		}
		return orchkit.Output{"deleted_count": result.DeletedCount}, nil

	case "count":
		n, err := coll.CountDocuments(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("mongodb: count: %w", err)
		}
		return orchkit.Output{"count": n}, nil

	default:
		return nil, fmt.Errorf("mongodb: unknown action %q", action)
	}
}
