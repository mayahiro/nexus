package comparecmd

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func jevIdentityTestRequest(t *testing.T) jevIdentityRequest {
	t.Helper()
	comparison := identityTestComparison()
	args := identityTestArguments(t, comparison)
	tasks, err := buildIdentityTasks(comparison, args)
	if err != nil {
		t.Fatal(err)
	}
	return buildJevIdentityRequest(tasks[0], args.Model)
}

func jevIdentityTestClient(t *testing.T, handler http.HandlerFunc) *jevIdentityClient {
	t.Helper()
	server := httptest.NewTestServer(t, handler)
	client, err := newJevIdentityClient("test-api-key")
	if err != nil {
		t.Fatal(err)
	}
	client.client.Transport = server.Client().Transport
	return client
}

func TestJevIdentityHTTPContract(t *testing.T) {
	request := jevIdentityTestRequest(t)
	client := jevIdentityTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-api-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected HTTP envelope: %s %s", r.Method, r.URL.Path)
		}
		var received jevIdentityRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if received.Model != defaultIdentityModel || received.State.Old.Node.Label != "Order 123" || len(received.Questions) != 3 {
			t.Errorf("unexpected request: %+v", received)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(identityTestResponse(received, "candidate_1", 0.99, 0.98))
	})
	response, err := client.evaluate(context.Background(), request)
	if err != nil || response.Model != defaultIdentityModel || *response.Answers["same_candidate_1"].Noul != 0.98 {
		t.Fatalf("response: %+v, %v", response, err)
	}
}

func TestJevIdentityRejectsMalformedAnswers(t *testing.T) {
	request := jevIdentityTestRequest(t)
	for _, test := range []struct {
		name   string
		mutate func(*jevIdentityResponse)
	}{
		{"missing answer", func(response *jevIdentityResponse) { delete(response.Answers, "same_candidate_1") }},
		{"extra answer", func(response *jevIdentityResponse) { response.Answers["extra"] = jevIdentityAnswer{} }},
		{"wrong model", func(response *jevIdentityResponse) { response.Model = "jev-1.14.0" }},
		{"invalid model", func(response *jevIdentityResponse) { response.Model = "jev-private\ntext" }},
		{"missing usage", func(response *jevIdentityResponse) { response.Usage = nil }},
		{"negative usage", func(response *jevIdentityResponse) { response.Usage.InputTokens = -1 }},
		{"missing noul", func(response *jevIdentityResponse) {
			response.Answers["same_candidate_1"] = jevIdentityAnswer{Type: "noul"}
		}},
		{"invalid noul", func(response *jevIdentityResponse) {
			response.Answers["same_candidate_1"] = jevIdentityAnswer{Type: "noul", Noul: identityTestNumber(1.1)}
		}},
		{"wrong type", func(response *jevIdentityResponse) {
			answer := response.Answers["correspondence"]
			answer.Type = "score"
			response.Answers["correspondence"] = answer
		}},
		{"missing confidence", func(response *jevIdentityResponse) {
			answer := response.Answers["correspondence"]
			answer.Confidence = nil
			response.Answers["correspondence"] = answer
		}},
		{"NaN confidence", func(response *jevIdentityResponse) {
			answer := response.Answers["correspondence"]
			answer.Confidence = identityTestNumber(math.NaN())
			response.Answers["correspondence"] = answer
		}},
		{"unknown choice", func(response *jevIdentityResponse) {
			answer := response.Answers["correspondence"]
			answer.Choice = "invented"
			response.Answers["correspondence"] = answer
		}},
		{"extra option", func(response *jevIdentityResponse) {
			response.Answers["correspondence"].Probabilities["invented"] = identityTestNumber(0)
		}},
		{"null probability", func(response *jevIdentityResponse) {
			response.Answers["correspondence"].Probabilities[identityNoMatch] = nil
		}},
		{"negative probability", func(response *jevIdentityResponse) {
			response.Answers["correspondence"].Probabilities[identityNoMatch] = identityTestNumber(-0.1)
		}},
		{"infinite probability", func(response *jevIdentityResponse) {
			response.Answers["correspondence"].Probabilities[identityNoMatch] = identityTestNumber(math.Inf(1))
		}},
		{"unnormalized probabilities", func(response *jevIdentityResponse) {
			response.Answers["correspondence"].Probabilities[identityNoMatch] = identityTestNumber(0.1)
		}},
		{"choice is not winner", func(response *jevIdentityResponse) {
			response.Answers["correspondence"].Probabilities[identityNoMatch] = identityTestNumber(1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := identityTestResponse(request, "candidate_1", 0.99, 0.99)
			test.mutate(&response)
			if err := validateJevIdentityResponse(request, response); err == nil {
				t.Fatal("malformed response was accepted")
			}
		})
	}
	request.Model = "jev-latest"
	response := identityTestResponse(request, "candidate_1", 0.99, 0.99)
	if err := validateJevIdentityResponse(request, response); err != nil {
		t.Fatalf("versioned result behind an alias was rejected: %v", err)
	}
}

func TestJevIdentityDoesNotExposeErrorBodiesOrFollowRedirects(t *testing.T) {
	request := jevIdentityTestRequest(t)
	for _, status := range []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := jevIdentityTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/do-not-follow")
				w.WriteHeader(status)
				w.Write([]byte("DO_NOT_ECHO_PRIVATE test-api-key"))
			})
			_, err := client.evaluate(context.Background(), request)
			if err == nil || strings.Contains(err.Error(), "DO_NOT_ECHO_PRIVATE") || strings.Contains(err.Error(), "test-api-key") || calls != 1 {
				t.Fatalf("unsafe error or redirected request: calls=%d, error=%v", calls, err)
			}
		})
	}
}

func TestJevIdentityRetriesOnlyTransientProviderStatuses(t *testing.T) {
	request := jevIdentityTestRequest(t)
	calls := 0
	client := jevIdentityTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(529)
		default:
			json.NewEncoder(w).Encode(identityTestResponse(request, "candidate_1", 0.99, 0.99))
		}
	})
	if _, err := client.evaluate(context.Background(), request); err != nil || calls != 3 {
		t.Fatalf("retry result: calls=%d, error=%v", calls, err)
	}
	if delay := jevIdentityRetryDelay("10", 0); delay != 10*time.Second {
		t.Fatalf("Retry-After was not honored: %v", delay)
	}
	if delay := jevIdentityRetryDelay("bad", 1); delay != time.Second {
		t.Fatalf("missing exponential fallback: %v", delay)
	}
}

func TestJevIdentityCancellationDuringRetry(t *testing.T) {
	request := jevIdentityTestRequest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := jevIdentityTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
		cancel()
	})
	if _, err := client.evaluate(ctx, request); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancellation was not preserved: calls=%d, error=%v", calls, err)
	}
}

func TestJevIdentityBoundsBodiesAndRejectsInvalidJSON(t *testing.T) {
	request := jevIdentityTestRequest(t)
	for _, body := range []string{`{"private":"DO_NOT_ECHO_PRIVATE"}`, `{`, strings.Repeat("x", jevIdentityMaxResponseBytes+1)} {
		client := jevIdentityTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		if _, err := client.evaluate(context.Background(), request); err == nil || strings.Contains(err.Error(), "DO_NOT_ECHO_PRIVATE") {
			t.Fatalf("invalid body result: %v", err)
		}
	}
	request.State.Old.Node.Text = strings.Repeat("x", jevIdentityMaxRequestBytes)
	client := jevIdentityTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("oversized request reached provider") })
	if _, err := client.evaluate(context.Background(), request); err == nil {
		t.Fatal("oversized request was accepted")
	}
}
