package handler

import (
	"encoding/json"
	"net/http"

	"github.com/packethost/pkg/log"
	"github.com/tinkerbell/hegel/internal/build"
)

func versionHandler(logger log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := struct {
			// Use git_rev to match the health endpoint reporting.
			Revision string `json:"git_rev"`
		}{
			Revision: build.GetGitRevision(),
		}

		encoder := json.NewEncoder(w)

		if err := encoder.Encode(payload); err != nil {
			logger.Error(err, "marshalling version")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}
