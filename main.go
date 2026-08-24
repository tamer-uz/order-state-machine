package main

import (
	"log"
	"net/http"
	"os"

	"github.com/tamer-uz/order-state-machine/api"
	"github.com/tamer-uz/order-state-machine/completion"
	"github.com/tamer-uz/order-state-machine/payment"
	"github.com/tamer-uz/order-state-machine/storage"
)

func main() {
	scenario := os.Getenv("SCENARIO")
	switch scenario {
	case "", "happy", "decline", "completion_failure", "void_failure":
	default:
		log.Fatalf("unknown SCENARIO %q", scenario)
	}

	paymentProvider := &payment.Stub{
		AuthorizeFails: scenario == "decline",
		VoidFails:      scenario == "void_failure",
	}
	completionProvider := &completion.Stub{
		Fails: scenario == "completion_failure" || scenario == "void_failure",
	}

	store := storage.New()
	handler := api.New(store, paymentProvider, completionProvider)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", handler.CreateOrder)
	mux.HandleFunc("GET /orders/{id}", handler.GetOrder)
	mux.HandleFunc("POST /orders/{id}/transitions", handler.ProcessTransition)

	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
