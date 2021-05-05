package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"

	"github.com/go-chi/chi/v5"
)

// Init configures and returns a chi router.
func InitRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/images/{filename}", apiReturnImage())
	return r
}

func apiReturnImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := chi.URLParam(r, "filename")

		data, err := os.ReadFile(fmt.Sprintf("savedfiles/%s", filename))
		if err != nil {
			w.Write([]byte("404 file not found"))
			return
		}

		// Find the image format.
		fmtReg := regexp.MustCompile(`\.(?:jpg|png)$`)
		format := fmtReg.FindString(filename)

		// Sets and writes content-type of 'application/json'.
		w.Header().Set("Content-Type", fmt.Sprintf("image/%s", format))
		w.Write(data)
	}
}
