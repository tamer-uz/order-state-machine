package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/tamer-uz/order-state-machine/models"
)

type Handler struct {
	store              OrderStore
	paymentProvider    PaymentProvider
	completionProvider CompletionProvider
}

func New(store OrderStore, paymentProvider PaymentProvider, completionProvider CompletionProvider) *Handler {
	return &Handler{
		store:              store,
		paymentProvider:    paymentProvider,
		completionProvider: completionProvider,
	}
}

type createOrderRequest struct {
	AmountCents int `json:"amount_cents"`
}

type transitionRequest struct {
	Command string `json:"command"`
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	if req.AmountCents <= 0 {
		writeError(w, http.StatusBadRequest, "amount_cents must be positive")
		return
	}

	order := models.NewOrder(req.AmountCents)
	h.store.UpsertOrder(order)
	writeJSON(w, http.StatusCreated, order)
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	order, found := h.store.GetOrder(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// Clients send commands. Events are derived from what the providers return.
func (h *Handler) ProcessTransition(w http.ResponseWriter, r *http.Request) {
	order, found := h.store.GetOrder(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	var req transitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	command := models.Command(req.Command)
	if !models.ValidCommands[command] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown command %q", req.Command))
		return
	}

	switch command {
	case models.AuthorizePayment:
		h.authorizePayment(w, order)
	case models.CompleteOrder:
		h.completeOrder(w, order)
	default:
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("command %q is accepted but not handled", command))
	}
}

// State is checked first: never charge a card we can't record the result on.
//
// Two mechanisms, each covering what the other cannot. Claiming the order into
// authorizing is what keeps a second request away from the provider; saving
// that claim conditionally is what stops two requests from both winning it.
func (h *Handler) authorizePayment(w http.ResponseWriter, order models.Order) {
	if order.CurrentState != models.Initialized {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("cannot authorize payment from state %q", order.CurrentState))
		return
	}

	// order is our own copy; nothing is visible to other requests until the save.
	if !h.apply(w, &order, models.EnteringAuthorization, nil) {
		return
	}
	if err := h.store.SaveIfStateIs(models.Initialized, order); err != nil {
		if errors.Is(err, models.ErrStateChanged) {
			writeError(w, http.StatusConflict,
				"cannot authorize payment: order state changed concurrently")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	authErr := h.paymentProvider.Authorize(order.ID, order.AmountCents)

	event := models.PaymentSucceeded
	if authErr != nil {
		event = models.PaymentFailed
	}
	if !h.apply(w, &order, event, authErr) {
		return
	}

	// Unconditional: the claim above is a lock, and nothing else can leave
	// authorizing, so no other request can have written since.
	h.store.UpsertOrder(order)
	writeJSON(w, http.StatusOK, order)
}

// Completion failure compensates with a void. A failed void surfaces in history.
func (h *Handler) completeOrder(w http.ResponseWriter, order models.Order) {
	if order.CurrentState != models.PaymentAuthorized {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("cannot complete order from state %q", order.CurrentState))
		return
	}

	if completeErr := h.completionProvider.Complete(order.ID); completeErr == nil {
		if !h.apply(w, &order, models.CompletionSucceeded, nil) {
			return
		}
	} else {
		if !h.apply(w, &order, models.CompletionFailed, completeErr) {
			return
		}

		voidErr := h.paymentProvider.Void(order.ID)

		event := models.VoidSucceeded
		if voidErr != nil {
			event = models.VoidFailed
		}
		if !h.apply(w, &order, event, voidErr) {
			return
		}
	}

	h.store.UpsertOrder(order)
	writeJSON(w, http.StatusOK, order)
}

// Callers pre-check state, so a rejection here is our bug, not the client's.
func (h *Handler) apply(w http.ResponseWriter, order *models.Order, event models.Event, cause error) bool {
	if err := order.ProcessTransition(event, cause); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
