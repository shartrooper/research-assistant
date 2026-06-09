package concierge

import "net/http"

// NewArtifactHandler returns an http.Handler that serves artifacts from the given directory with CORS support.
func NewArtifactHandler(artifactDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(artifactDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
