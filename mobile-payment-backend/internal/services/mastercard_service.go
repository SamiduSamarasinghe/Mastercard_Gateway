package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mobile-payment-backend/internal/config"
	"mobile-payment-backend/internal/models"
	"mobile-payment-backend/internal/repositories"
)

type GatewayService interface {
	CreateSession(order *models.Order, authLimit int) (*models.Session, error)
	UpdateSession(sessionID, orderID, amount, currency string) error
	ProcessPayment(request *models.PaymentRequest) (*models.PaymentResponse, error)
	CreateToken(sessionID string) (string, error)
}

type gatewayService struct {
	cfg             *config.Config
	sessionRepo     repositories.SessionRepository
	transactionRepo repositories.TransactionRepository
	tokenRepo       repositories.TokenRepository
	httpClient      *http.Client
}

func NewGatewayService(
	cfg *config.Config,
	sessionRepo repositories.SessionRepository,
	transactionRepo repositories.TransactionRepository,
	tokenRepo repositories.TokenRepository,
) GatewayService {
	return &gatewayService{
		cfg:             cfg,
		sessionRepo:     sessionRepo,
		transactionRepo: transactionRepo,
		tokenRepo:       tokenRepo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateToken creates a payment token from a session (for card-on-file)
func (s *gatewayService) CreateToken(sessionID string) (string, error) {
	endpoint := fmt.Sprintf("/api/rest/version/%s/merchant/%s/token",
		s.cfg.APIVersion, s.cfg.MastercardMerchantID)

	// Using session for tokenization
	request := map[string]interface{}{
		"session": map[string]interface{}{
			"id": sessionID,
		},
		"sourceOfFunds": map[string]interface{}{
			"type": "CARD",
		},
	}

	body, err := s.makeRequest("POST", endpoint, request)
	if err != nil {
		return "", fmt.Errorf("failed to create token: %v", err)
	}

	var response struct {
		Token   string `json:"token"`
		Success bool   `json:"success"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse token response: %v", err)
	}

	if !response.Success {
		return "", fmt.Errorf("gateway failed to create token")
	}

	return response.Token, nil
}

// CreateSession creates a new payment session in Mastercard Gateway
func (s *gatewayService) CreateSession(order *models.Order, authLimit int) (*models.Session, error) {
	endpoint := fmt.Sprintf("/api/rest/version/%s/merchant/%s/session",
		s.cfg.APIVersion, s.cfg.MastercardMerchantID)

	request := map[string]interface{}{
		// "authenticationLimit": authLimit,
	}

	body, err := s.makeRequest("POST", endpoint, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	var response struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Result string `json:"result"` // ✅ CORRECT FIELD
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse session response: %v", err)
	}

	if response.Result != "SUCCESS" { // ✅ CORRECT CHECK
		return nil, fmt.Errorf("gateway failed to create session. result: %s", response.Result)
	}

	session := &models.Session{
		GatewayID:  response.Session.ID,
		OrderID:    order.ReferenceID,
		Amount:     order.Amount,
		Currency:   order.Currency,
		Status:     "created",
		APIVersion: s.cfg.APIVersion,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(30 * time.Minute), // Sessions expire in 30 mins
	}

	return session, nil
}

// UpdateSession updates session with order details (called from mobile SDK)
func (s *gatewayService) UpdateSession(sessionID, orderID, amount, currency string) error {
	endpoint := fmt.Sprintf("/api/rest/version/%s/merchant/%s/session/%s",
		s.cfg.APIVersion, s.cfg.MastercardMerchantID, sessionID)

	request := map[string]interface{}{
		"order": map[string]interface{}{
			"id":       orderID,
			"amount":   amount,
			"currency": currency,
		},
		"authentication": map[string]interface{}{
			"acceptVersions": "3DS2",
			"channel":        "PAYER_APP",
			"purpose":        "PAYMENT_TRANSACTION",
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return fmt.Errorf("failed to update session: %v", err)
	}

	// If response is empty, assume success
	if len(body) == 0 {
		fmt.Println("DEBUG: UpdateSession - Empty response (assumed success)")
		return nil
	}

	// Try to parse as JSON
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		fmt.Printf("DEBUG: UpdateSession - Non-JSON response: %s\n", string(body))
		return nil // Assume success if not JSON
	}

	// Check for result field
	if result, ok := response["result"].(string); ok {
		if result != "SUCCESS" {
			return fmt.Errorf("update session failed with result: %s", result)
		}
	}

	return nil
}

// ProcessPayment processes payment using session ID
func (s *gatewayService) ProcessPayment(request *models.PaymentRequest) (*models.PaymentResponse, error) {
	// Generate a simple order ID for Gateway
	gatewayOrderID := fmt.Sprintf("ORDER%d", time.Now().UnixNano())

	endpoint := fmt.Sprintf("/api/rest/version/%s/merchant/%s/order/%s/transaction/1",
		s.cfg.APIVersion, s.cfg.MastercardMerchantID, gatewayOrderID)

	payload := map[string]interface{}{
		"apiOperation": request.Operation,
		"session": map[string]interface{}{
			"id": request.SessionID,
		},
		"order": map[string]interface{}{
			// "id":       gatewayOrderID, // Use same ID here
			"amount":   request.Amount,
			"currency": request.Currency,
		},
		"sourceOfFunds": map[string]interface{}{
			"type": "CARD",
		},
	}

	// // Add order details if provided
	// if amount != "" && currency != "" {
	// 	payload["order"] = map[string]interface{}{
	// 		"amount":   amount,
	// 		"currency": currency,
	// 	}
	// }

	body, err := s.makeRequest("PUT", endpoint, payload)
	if err != nil {
		return nil, fmt.Errorf("payment failed: %v", err)
	}

	var gatewayResp map[string]interface{}
	if err := json.Unmarshal(body, &gatewayResp); err != nil {
		return nil, fmt.Errorf("failed to parse payment response: %v", err)
	}

	response := &models.PaymentResponse{
		Success:         gatewayResp["result"] == "SUCCESS",
		GatewayCode:     getString(gatewayResp, "gatewayCode"),
		TransactionID:   getString(gatewayResp, "transaction.id"),
		OrderID:         gatewayOrderID, // Use the generated order ID
		Status:          getString(gatewayResp, "transaction.status"),
		Recommendation:  getString(gatewayResp, "response.gatewayRecommendation"),
		GatewayResponse: gatewayResp,
	}

	// Parse amount
	if amountVal, ok := gatewayResp["order"].(map[string]interface{})["amount"]; ok {
		switch amt := amountVal.(type) {
		case float64:
			response.Amount = amt
		case string:
			if parsedAmt, err := strconv.ParseFloat(amt, 64); err == nil {
				response.Amount = parsedAmt
			}
		}
	}

	if curr, ok := gatewayResp["order"].(map[string]interface{})["currency"]; ok {
		response.Currency = curr.(string)
	}

	return response, nil
}

// Helper method for API requests
func (s *gatewayService) makeRequest(method, endpoint string, payload interface{}) ([]byte, error) {
	url := fmt.Sprintf("https://%s%s", s.cfg.MastercardHost, endpoint)

	fmt.Printf("DEBUG: Making request to: %s\n", url) // ADD THIS
	fmt.Printf("DEBUG: Merchant ID: %s\n", s.cfg.MastercardMerchantID)

	var body []byte
	var err error

	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %v", err)
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Basic Auth
	auth := fmt.Sprintf("merchant.%s:%s", s.cfg.MastercardMerchantID, s.cfg.MastercardAPIPassword)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	req.Header.Set("Authorization", "Basic "+encodedAuth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	fmt.Printf("DEBUG: Response Status: %d\n", resp.StatusCode) // ADD THIS
	fmt.Printf("DEBUG: Response Body: %s\n", string(respBody))

	return respBody, nil
}

// Helper to safely get string from map
func getString(m map[string]interface{}, path string) string {
	keys := strings.Split(path, ".")
	var current interface{} = m

	for _, key := range keys {
		if currentMap, ok := current.(map[string]interface{}); ok {
			if val, exists := currentMap[key]; exists {
				current = val
			} else {
				return ""
			}
		} else {
			return ""
		}
	}

	if str, ok := current.(string); ok {
		return str
	}
	return ""
}
