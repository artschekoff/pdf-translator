package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// purchaseOut matches GET /billing/purchases response from the payments-service.
type purchaseOut struct {
	ID                      string `json:"id"`
	UserID                  string `json:"user_id"`
	PriceID                 string `json:"price_id"`
	ProviderPaymentIntentID string `json:"provider_payment_intent_id"`
}

type Service struct {
	repo            *Repository
	paymentsBaseURL string
	httpClient      *http.Client
}

func NewService(repo *Repository, paymentsBaseURL string) *Service {
	return &Service{
		repo:            repo,
		paymentsBaseURL: strings.TrimRight(paymentsBaseURL, "/"),
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateCheckoutSession proxies to the payments-service and returns the Stripe checkout URL.
func (s *Service) CreateCheckoutSession(ctx context.Context, planID, bearerToken string) (string, error) {
	plans, err := s.repo.ListPlans()
	if err != nil {
		return "", err
	}
	var plan *Plan
	for i := range plans {
		if plans[i].ID == planID {
			plan = &plans[i]
			break
		}
	}
	if plan == nil {
		return "", fmt.Errorf("plan not found")
	}
	if plan.PaymentsPriceID == nil || *plan.PaymentsPriceID == "" {
		return "", fmt.Errorf("plan has no linked payments-service price — contact support")
	}

	body, _ := json.Marshal(map[string]string{"price_id": *plan.PaymentsPriceID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.paymentsBaseURL+"/billing/checkout", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("payments service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("payments service error: %s", resp.Status)
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.URL, nil
}

// SyncPurchases fetches the user's purchases from the payments-service and credits
// any pages that haven't been credited yet. Returns the number of pages just credited.
func (s *Service) SyncPurchases(ctx context.Context, userID, bearerToken string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.paymentsBaseURL+"/billing/purchases", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("payments service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("payments service error: %s", resp.Status)
	}

	var purchases []purchaseOut
	if err := json.NewDecoder(resp.Body).Decode(&purchases); err != nil {
		return 0, err
	}

	totalCredited := 0
	for _, p := range purchases {
		credited, err := s.repo.IsPurchaseCredited(p.ID)
		if err != nil {
			log.Printf("billing sync: checking purchase %s: %v", p.ID, err)
			continue
		}
		if credited {
			continue
		}

		plan, err := s.repo.PlanByPaymentsPriceID(p.PriceID)
		if err != nil {
			log.Printf("billing sync: looking up plan for price %s: %v", p.PriceID, err)
			continue
		}
		if plan == nil {
			log.Printf("billing sync: no plan found for price %s — skipping", p.PriceID)
			continue
		}

		if err := s.repo.AddPages(userID, plan.Pages); err != nil {
			log.Printf("billing sync: crediting pages for user %s: %v", userID, err)
			continue
		}
		if err := s.repo.RecordCreditedPurchase(p.ID, userID, plan.Pages); err != nil {
			log.Printf("billing sync: recording purchase %s: %v", p.ID, err)
		}
		totalCredited += plan.Pages
		log.Printf("billing: credited %d pages to user %s (purchase %s)", plan.Pages, userID, p.ID)
	}

	return totalCredited, nil
}

func (s *Service) GetBalance(userID string) (int, error) {
	return s.repo.GetBalance(userID)
}

func (s *Service) ListPlans() ([]Plan, error) {
	return s.repo.ListPlans()
}
