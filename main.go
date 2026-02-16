package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/docker/docker/daemon/logger"
	"github.com/docker/go-plugins-helpers/sdk"
	log "github.com/sirupsen/logrus"

	axiomDriver "github.com/ashutoshpw/axiom-docker-logger/driver"
	"github.com/axiomhq/axiom-go/axiom"
)

const socketAddress = "/run/docker/plugins/axiom.sock"

type startLoggingRequest struct {
	File string      `json:"File"`
	Info logger.Info `json:"Info"`
}

type stopLoggingRequest struct {
	File string `json:"File"`
}

type capabilitiesResponse struct {
	Cap logger.Capability `json:"Cap"`
}

type response struct {
	Err string `json:"Err"`
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	// Read configuration from environment variables.
	// These are set via `docker plugin set` or in config.json defaults.
	token := os.Getenv("AXIOM_TOKEN")
	dataset := os.Getenv("AXIOM_DATASET")
	axiomURL := os.Getenv("AXIOM_URL")

	if token == "" {
		log.Fatal("AXIOM_TOKEN environment variable is required")
	}
	if dataset == "" {
		dataset = "docker-logs"
	}
	if axiomURL == "" {
		axiomURL = "https://api.axiom.co"
	}

	// Initialize the Axiom client.
	opts := []axiom.Option{
		axiom.SetToken(token),
		axiom.SetURL(axiomURL),
	}
	client, err := axiom.NewClient(opts...)
	if err != nil {
		log.WithError(err).Fatal("Failed to create Axiom client")
	}

	log.WithFields(log.Fields{
		"dataset":  dataset,
		"axiomURL": axiomURL,
	}).Info("Axiom Docker logging plugin starting")

	// Create the driver.
	d := axiomDriver.New(client, dataset)

	// Create the plugin handler.
	h := sdk.NewHandler(`{"Implements": ["LoggingDriver"]}`)
	setupHandlers(&h, d)

	// Serve on the Unix socket.
	log.WithField("socket", socketAddress).Info("Listening")
	if err := h.ServeUnix(socketAddress, 0); err != nil {
		log.WithError(err).Fatal("Failed to serve")
	}
}

func setupHandlers(h *sdk.Handler, d *axiomDriver.Driver) {
	h.HandleFunc("/LogDriver.StartLogging", func(w http.ResponseWriter, r *http.Request) {
		var req startLoggingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err := d.StartLogging(req.File, req.Info)
		respond(err, w)
	})

	h.HandleFunc("/LogDriver.StopLogging", func(w http.ResponseWriter, r *http.Request) {
		var req stopLoggingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err := d.StopLogging(req.File)
		respond(err, w)
	})

	h.HandleFunc("/LogDriver.Capabilities", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(&capabilitiesResponse{
			Cap: logger.Capability{ReadLogs: false},
		})
	})

	h.HandleFunc("/LogDriver.ReadLogs", func(w http.ResponseWriter, r *http.Request) {
		// ReadLogs is not implemented yet.
		// Return an empty response so Docker doesn't error out.
		json.NewEncoder(w).Encode(&response{Err: "reading logs is not supported"})
	})
}

func respond(err error, w http.ResponseWriter) {
	var res response
	if err != nil {
		res.Err = err.Error()
	}
	json.NewEncoder(w).Encode(&res)
}
