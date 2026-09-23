package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tamer-uz/order-state-machine/completion"
	"github.com/tamer-uz/order-state-machine/models"
	"github.com/tamer-uz/order-state-machine/payment"
	"github.com/tamer-uz/order-state-machine/storage"
)

// newTestServer wires a handler with stubs configured for one scenario.
func newTestServer(authFails, completionFails, voidFails bool) *httptest.Server {
	return serve(New(
		storage.New(),
		&payment.Stub{AuthorizeFails: authFails, VoidFails: voidFails},
		&completion.Stub{Fails: completionFails},
	))
}

func serve(h *Handler) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", h.CreateOrder)
	mux.HandleFunc("GET /orders/{id}", h.GetOrder)
	mux.HandleFunc("POST /orders/{id}/transitions", h.ProcessTransition)

	return httptest.NewServer(mux)
}

// postStatus returns transport errors rather than failing the test, so it is
// safe to call from a goroutine. t.Fatalf outside the test goroutine calls
// runtime.Goexit on the wrong stack and hangs the run instead of failing it.
func postStatus(url string, body any) (int, models.Order, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, models.Order{}, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return 0, models.Order{}, err
	}
	defer resp.Body.Close()

	var order models.Order
	json.NewDecoder(resp.Body).Decode(&order)
	return resp.StatusCode, order, nil
}

func post(t *testing.T, url string, body any) (int, models.Order) {
	t.Helper()

	status, order, err := postStatus(url, body)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	return status, order
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

func sendCommandStatus(srv *httptest.Server, id string, command models.Command) (int, models.Order, error) {
	return postStatus(srv.URL+"/orders/"+id+"/transitions",
		map[string]models.Command{"command": command})
}

func sendCommand(t *testing.T, srv *httptest.Server, id string, command models.Command) (int, models.Order) {
	t.Helper()

	status, order, err := sendCommandStatus(srv, id, command)
	if err != nil {
		t.Fatalf("send %q: %v", command, err)
	}
	return status, order
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
	// initialized, authorizing, payment_authorized
	if len(fetched.StateTransitionHistory) != 3 {
		t.Errorf("history has %d entries, want 3", len(fetched.StateTransitionHistory))
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

// countingPayment records how many times the provider was actually reached.
type countingPayment struct {
	mu         sync.Mutex
	authorized int
}

func (p *countingPayment) Authorize(orderID string, cents int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.authorized++
	return nil
}

func (p *countingPayment) Void(orderID string) error { return nil }

func (p *countingPayment) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.authorized
}

// barrierStore holds the first want readers inside GetOrder until all of them
// have read, so every caller leaves the read with the same stale order. That is
// the interleaving a conditional write has to defeat; relying on timing to
// produce it does not work, because the read and the claim are microseconds
// apart and the requests end up serialising by luck.
type barrierStore struct {
	*storage.Store

	want    int
	release chan struct{}
	mu      sync.Mutex
	arrived int
}

func newBarrierStore(want int) *barrierStore {
	return &barrierStore{
		Store:   storage.New(),
		want:    want,
		release: make(chan struct{}),
	}
}

func (s *barrierStore) GetOrder(id string) (models.Order, bool) {
	order, found := s.Store.GetOrder(id)

	s.mu.Lock()
	arrived := s.arrived
	if arrived < s.want {
		s.arrived++
	}
	s.mu.Unlock()

	switch {
	case arrived == s.want-1:
		close(s.release)
	case arrived < s.want-1:
		select {
		case <-s.release:
		case <-time.After(5 * time.Second):
			panic("barrierStore: not all readers arrived")
		}
	}
	return order, found
}

func TestConcurrentAuthorizeChargesOnce(t *testing.T) {
	const callers = 20

	provider := &countingPayment{}
	srv := serve(New(newBarrierStore(callers), provider, &completion.Stub{}))
	defer srv.Close()

	order := createOrder(t, srv)

	statuses := make([]int, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i], _, errs[i] = sendCommandStatus(srv, order.ID, models.AuthorizePayment)
		}()
	}
	wg.Wait()

	// Reported here, not in the goroutines, for the reason postStatus documents.
	for _, err := range errs {
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
	}

	var ok, conflict int
	for _, status := range statuses {
		switch status {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Errorf("unexpected status %d", status)
		}
	}

	if ok != 1 {
		t.Errorf("%d callers got 200, want exactly 1", ok)
	}
	if conflict != callers-1 {
		t.Errorf("%d callers got 409, want %d", conflict, callers-1)
	}
	if got := provider.count(); got != 1 {
		t.Errorf("provider authorized %d times, want 1", got)
	}

	_, fetched := get(t, srv.URL+"/orders/"+order.ID)
	if fetched.CurrentState != models.PaymentAuthorized {
		t.Errorf("final state = %q, want %q", fetched.CurrentState, models.PaymentAuthorized)
	}
}
