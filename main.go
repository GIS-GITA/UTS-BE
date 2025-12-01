package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- STRUKTUR DATA UNTUK GEOJSON ---

// Geometry mendefinisikan struktur geometri untuk Point GeoJSON
type Geometry struct {
	Type        string    `json:"type" bson:"type"`
	Coordinates []float64 `json:"coordinates" bson:"coordinates"` // [longitude, latitude]
}

// Properties berisi data non-spasial dari sebuah fitur
type Properties struct {
	Name        string `json:"name" bson:"name"`
	Description string `json:"description" bson:"description"`
}

// LocationFeature adalah representasi lengkap dari sebuah fitur GeoJSON
type LocationFeature struct {
	ID         primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Type       string             `json:"type" bson:"type"`
	Properties Properties         `json:"properties" bson:"properties"`
	Geometry   Geometry           `json:"geometry" bson:"geometry"`
}

// FeatureCollection adalah wrapper untuk mengembalikan array dari fitur
type FeatureCollection struct {
	Type     string            `json:"type"`
	Features []LocationFeature `json:"features"`
}

var collection *mongo.Collection

// --- HANDLERS ---

// createLocation - Membuat lokasi baru (POST)
func createLocation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var feature LocationFeature
	if err := json.NewDecoder(r.Body).Decode(&feature); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	feature.Type = "Feature" // Pastikan tipe adalah Feature

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := collection.InsertOne(ctx, feature)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(result)
}

// getLocations - Mengambil semua lokasi (GET)
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

	if err = cursor.All(ctx, &features); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Bungkus dalam FeatureCollection
	featureCollection := FeatureCollection{
		Type:     "FeatureCollection",
		Features: features,
	}

	json.NewEncoder(w).Encode(featureCollection)
}

// updateLocation - Memperbarui lokasi (PUT)
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

// deleteLocation - Menghapus lokasi (DELETE)
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

// --- MAIN FUNCTION ---

func main() {
	// Koneksi ke MongoDB
	log.Println("Connecting to MongoDB...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb+srv://mhaitsamia:Ebg4K8HzMEWOCESZ@presensi.g9kkirr.mongodb.net/"))
	if err != nil {
		log.Fatal(err)
	}

	collection = client.Database("gis_db").Collection("locations")

	// PENTING: Membuat 2dsphere index untuk query geospasial
	indexModel := mongo.IndexModel{
		Keys: bson.M{"geometry": "2dsphere"},
	}
	_, err = collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Println("Could not create 2dsphere index, it might already exist.")
	} else {
		log.Println("2dsphere index created successfully on 'geometry' field.")
	}

	log.Println("Connected to MongoDB!")

	// Router
	r := mux.NewRouter()
	r.HandleFunc("/locations", getLocations).Methods("GET")
	r.HandleFunc("/locations", createLocation).Methods("POST")
	r.HandleFunc("/locations/{id}", updateLocation).Methods("PUT")
	r.HandleFunc("/locations/{id}", deleteLocation).Methods("DELETE")

	// CORS Middleware
	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"})
	originsOk := handlers.AllowedOrigins([]string{"*"})
	methodsOk := handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})

	// Start Server
	log.Println("Server is running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", handlers.CORS(originsOk, headersOk, methodsOk)(r)))
}