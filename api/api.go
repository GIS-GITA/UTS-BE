package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- STRUKTUR DATA ---
type Geometry struct {
	Type        string    `json:"type" bson:"type"`
	Coordinates []float64 `json:"coordinates" bson:"coordinates"`
}

type Properties struct {
	Name        string `json:"name" bson:"name"`
	Description string `json:"description" bson:"description"`
}

type LocationFeature struct {
	ID         primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Type       string             `json:"type" bson:"type"`
	Properties Properties         `json:"properties" bson:"properties"`
	Geometry   Geometry           `json:"geometry" bson:"geometry"`
}

type FeatureCollection struct {
	Type     string            `json:"type"`
	Features []LocationFeature `json:"features"`
}

// Global Variables
var (
	collection *mongo.Collection
	dbOnce     sync.Once
	router     *mux.Router
)

// --- FUNGSI KONEKSI DB (Aman dengan sync.Once) ---
func initDB() {
	dbOnce.Do(func() {
		// Load .env jika belum di-load di main
		godotenv.Load()

		// Gunakan Environment Variable untuk keamanan
		mongoURI := os.Getenv("MONGO_URI")
		if mongoURI == "" {
			log.Fatal("❌ MONGO_URI environment variable tidak ditemukan. Silakan set di .env atau environment")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
		if err != nil {
			log.Fatal("Gagal connect DB:", err)
		}

		collection = client.Database("gis_db").Collection("locations")

		// Create Index (Optional, best effort)
		indexModel := mongo.IndexModel{Keys: bson.M{"geometry": "2dsphere"}}
		collection.Indexes().CreateOne(ctx, indexModel)

		log.Println("✅ Connected to MongoDB!")
	})
}

// --- MIDDLEWARE CORS ---
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set header CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		// PENTING: Jika request adalah OPTIONS (preflight), langsung return OK tanpa lanjut ke handler lain
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --- HANDLERS ---
func createLocation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var feature LocationFeature
	if err := json.NewDecoder(r.Body).Decode(&feature); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	feature.Type = "Feature"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := collection.InsertOne(ctx, feature)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func getLocations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var features []LocationFeature
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	// Pastikan array tidak nil
	features = make([]LocationFeature, 0)
	if err = cursor.All(ctx, &features); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	featureCollection := FeatureCollection{
		Type:     "FeatureCollection",
		Features: features,
	}
	json.NewEncoder(w).Encode(featureCollection)
}

func updateLocation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}
	var feature LocationFeature
	if err := json.NewDecoder(r.Body).Decode(&feature); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"properties": feature.Properties,
			"geometry":   feature.Geometry,
		},
	}
	_, err = collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(feature)
}

func deleteLocation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if result.DeletedCount == 0 {
		http.Error(w, "Location not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "Location deleted successfully"})
}

// --- SETUP ROUTER (Dipanggil oleh main.go dan Handler Vercel) ---
func SetupRouter() *mux.Router {
	initDB() // Pastikan DB connect
	r := mux.NewRouter()

	// Terapkan CORS
	r.Use(corsMiddleware)

	r.HandleFunc("/locations", getLocations).Methods("GET", "OPTIONS")
	r.HandleFunc("/locations", createLocation).Methods("POST", "OPTIONS")
	r.HandleFunc("/locations/{id}", updateLocation).Methods("PUT", "OPTIONS")
	r.HandleFunc("/locations/{id}", deleteLocation).Methods("DELETE", "OPTIONS")

	return r
}

// --- VERCEL ENTRY POINT ---
// Fungsi ini yang akan dicari oleh Vercel di dalam file api/api.go
func Handler(w http.ResponseWriter, r *http.Request) {
	if router == nil {
		router = SetupRouter()
	}
	router.ServeHTTP(w, r)
}
