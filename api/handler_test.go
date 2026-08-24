package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamer-uz/order-state-machine/completion"
	"github.com/tamer-uz/order-state-machine/models"
	"github.com/tamer-uz/order-state-machine/payment"
	"github.com/tamer-uz/order-state-machine/storage"
)

// newTestServer wires a handler with stubs configured for one scenario.
func newTestServer(authFails, completionFails, voidFails bool) *httptest.Server {
	h := New(
		storage.New(),
		&payment.Stub{AuthorizeFails: authFails, VoidFails: voidFails},
		&completion.Stub{Fails: completionFails},
	)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", h.CreateOrder)
	mux.HandleFunc("GET /orders/{id}", h.GetOrder)
	mux.HandleFunc("POST /orders/{id}/transitions", h.ProcessTransition)

	return httptest.NewServer(mux)
}

func post(t *testing.T, url string, body any) (int, models.Order) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer resp.Body.Close()

	var order models.Order
	json.NewDecoder(resp.Body).Decode(&order)
	return resp.StatusCode, order
}

func get(t *testing.T, url string) (int, models.Order) {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()

	var order models.Order
	json.NewDecoder(resp.Body).Decode(&order)
	return resp.StatusCode, order
}

func createOrder(t *testing.T, srv *httptest.Server) models.Order {
	t.Helper()

	status, order := post(t, srv.URL+"/orders", map[string]int{"amount_cents": 5000})
	if status != http.StatusCreated {
		t.Fatalf("create order: status = %d, want %d", status, http.StatusCreated)
	}
	return order
}

func sendCommand(t *testing.T, srv *httptest.Server, id string, command models.Command) (int, models.Order) {
	t.Helper()

	return post(t, srv.URL+"/orders/"+id+"/transitions",
		map[string]models.Command{"command": command})
}

// assertReasonRecorded checks a failure was surfaced rather than swallowed.
func assertReasonRecorded(t *testing.T, order models.Order, state models.State) {
	t.Helper()

	for _, entry := range order.StateTransitionHistory {
		if entry.State == state {
			if entry.Reason == "" {
				t.Errorf("history entry %q has no reason recorded", state)
			}
			return
		}
	}
	t.Errorf("no history entry for state %q", state)
}

func TestOrderScenarios(t *testing.T) {
	cases := []struct {
		name            string
		authFails       bool
		completionFails bool
		voidFails       bool
		commands        []models.Command
		wantState       models.State
		wantReasonOn    models.State // state whose history entry must carry a reason
	}{
		{
			name:      "happy path",
			commands:  []models.Command{models.AuthorizePayment, models.CompleteOrder},
			wantState: models.Complete,
		},
		{
			name:         "payment declined",
			authFails:    true,
			commands:     []models.Command{models.AuthorizePayment},
			wantState:    models.Rejected,
			wantReasonOn: models.Rejected,
		},
		{
			name:            "completion fails, void succeeds",
			completionFails: true,
			commands:        []models.Command{models.AuthorizePayment, models.CompleteOrder},
			wantState:       models.Cancelled,
			wantReasonOn:    models.VoidPending,
		},
		{
			name:            "completion fails, void fails",
			completionFails: true,
			voidFails:       true,
			commands:        []models.Command{models.AuthorizePayment, models.CompleteOrder},
			wantState:       models.NeedsAttention,
			wantReasonOn:    models.NeedsAttention,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newTestServer(c.authFails, c.completionFails, c.voidFails)
			defer srv.Close()

			order := createOrder(t, srv)

			for _, command := range c.commands {
				var status int
				status, order = sendCommand(t, srv, order.ID, command)
				if status != http.StatusOK {
					t.Fatalf("command %q: status = %d, want %d", command, status, http.StatusOK)
				}
			}

			if order.CurrentState != c.wantState {
				t.Errorf("final state = %q, want %q", order.CurrentState, c.wantState)
			}

			if c.wantReasonOn != "" {
				assertReasonRecorded(t, order, c.wantReasonOn)
			}
		})
	}
}

func TestIllegalTransitionReturnsConflict(t *testing.T) {
	srv := newTestServer(false, false, false)
	defer srv.Close()

	order := createOrder(t, srv)

	// complete_order is only valid from payment_authorized.
	status, _ := sendCommand(t, srv, order.ID, models.CompleteOrder)

	if status != http.StatusConflict {
		t.Errorf("status = %d, want %d", status, http.StatusConflict)
	}
}

func TestAuthorizeFromWrongStateReturnsConflict(t *testing.T) {
	srv := newTestServer(false, false, false)
	defer srv.Close()

	order := createOrder(t, srv)
	sendCommand(t, srv, order.ID, models.AuthorizePayment)

	// Already authorized; a retry must not charge the card twice.
	status, _ := sendCommand(t, srv, order.ID, models.AuthorizePayment)

	if status != http.StatusConflict {
		t.Errorf("status = %d, want %d", status, http.StatusConflict)
	}
}

func TestUnknownCommandReturnsBadRequest(t *testing.T) {
	srv := newTestServer(false, false, false)
	defer srv.Close()

	order := createOrder(t, srv)

	// void_succeeded is a real event, but it is applied internally, never sent.
	status, _ := sendCommand(t, srv, order.ID, models.Command("void_succeeded"))

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestTransitionOnUnknownOrderReturnsNotFound(t *testing.T) {
	srv := newTestServer(false, false, false)
	defer srv.Close()

	status, _ := sendCommand(t, srv, "does-not-exist", models.AuthorizePayment)

	if status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", status, http.StatusNotFound)
	}
}

func TestGetOrderReturnsCurrentStateAndHistory(t *testing.T) {
	srv := newTestServer(false, false, false)
	defer srv.Close()

	created := createOrder(t, srv)
	sendCommand(t, srv, created.ID, models.AuthorizePayment)

	status, fetched := get(t, srv.URL+"/orders/"+created.ID)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if fetched.ID != created.ID {
		t.Errorf("id = %q, want %q", fetched.ID, created.ID)
	}
	if fetched.CurrentState != models.PaymentAuthorized {
		t.Errorf("state = %q, want %q", fetched.CurrentState, models.PaymentAuthorized)
	}
	if len(fetched.StateTransitionHistory) != 2 {
		t.Errorf("history has %d entries, want 2", len(fetched.StateTransitionHistory))
	}
}

func TestGetUnknownOrderReturnsNotFound(t *testing.T) {
	srv := newTestServer(false, false, false)
	defer srv.Close()

	status, _ := get(t, srv.URL+"/orders/does-not-exist")

	if status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", status, http.StatusNotFound)
	}
}
