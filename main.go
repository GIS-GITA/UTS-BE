package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
	// GANTI IMPORT INI SESUAI NAMA MODULE DI go.mod ANDA
	// Format: "NamaModuleAnda/NamaFolder"
	// Contoh jika nama module di go.mod adalah "github.com/ulbithebest/BE-pendaftaran"
	// maka importnya:
	"BE-GIS/api"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env file not found, using environment variables")
	}

	// Panggil SetupRouter dari folder api
	r := api.SetupRouter()

	log.Println("🚀 Server local berjalan di port 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
