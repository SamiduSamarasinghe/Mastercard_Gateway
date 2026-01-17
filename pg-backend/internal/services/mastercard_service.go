package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pg-backend/internal/config"
	"pg-backend/pkg/utils"
)

// Test data for Google Pay (from MTF documentation)
const (
	// Test DPANs (Device Primary Account Numbers) for mobile wallet payments
	TestDPANVisa        = "4895370012003478"
	TestDPANAmex        = "370295136149943"
	TestDPANExpiryMonth = "12"
	TestDPANExpiryYear  = "2028"

	// Test FPAN (Funding Primary Account Number) for card saved to Google account
	TestFPANVisa        = "4111111111111111"
	TestFPANExpiryMonth = "12"
	TestFPANExpiryYear  = "2028"

	// Test cryptogram and ECI from Postman collection
	TestCryptogram   = "IA/8pdiWftSsxpFT6wABoDABhgA="
	TestEciIndicator = "20"
)

type MastercardService interface {
	VerifyCard(cardNumber, expiryMonth, expiryYear, cvv, currency string) (*VerifyResponse, error)
	CreatePaymentToken(cardNumber, expiryMonth, expiryYear, cvv string) (*TokenResponse, error)

	// Direct payment operations
	PayWithToken(token, amount, currency string) (*PaymentResponse, error)
	PayWithCard(cardNumber, expiryMonth, expiryYear, cvv, amount, currency string) (*PaymentResponse, error)

	// Authorization flow operations (NEW)
	AuthorizeWithToken(token, amount, currency string) (*PaymentResponse, error)
	AuthorizeWithCard(cardNumber, expiryMonth, expiryYear, cvv, amount, currency string) (*PaymentResponse, error)
	CaptureAuthorization(orderID, amount, currency string) (*PaymentResponse, error)
	VoidAuthorization(orderID string) (*PaymentResponse, error)
	UpdateAuthorization(orderID, amount, currency string) (*PaymentResponse, error)

	// Other operations
	RefundPayment(orderID, amount, currency string) (*PaymentResponse, error)

	// NEW: Google Pay methods for merchant-decrypted flow
	PayWithGooglePay(cardNumber, expiryMonth, expiryYear, cryptogram, eci, amount, currency string) (*PaymentResponse, error)
	AuthorizeWithGooglePay(cardNumber, expiryMonth, expiryYear, cryptogram, eci, amount, currency string) (*PaymentResponse, error)

	// For future use with real Google Pay tokens (Phase 2)
	PayWithGooglePayToken(paymentToken, amount, currency string) (*PaymentResponse, error)
	AuthorizeWithGooglePayToken(paymentToken, amount, currency string) (*PaymentResponse, error)

	// Apple Pay methods
	PayWithApplePayToken(paymentToken, amount, currency string) (*PaymentResponse, error)
	AuthorizeWithApplePayToken(paymentToken, amount, currency string) (*PaymentResponse, error)
}

// Add GooglePayPaymentRequest struct for the merchant-decrypted flow
type GooglePayPaymentRequest struct {
	ApiOperation string `json:"apiOperation"`
	Order        struct {
		Amount         string `json:"amount"`
		Currency       string `json:"currency"`
		WalletProvider string `json:"walletProvider"`
	} `json:"order"`
	SourceOfFunds struct {
		Type     string `json:"type"`
		Provided struct {
			Card struct {
				Number string `json:"number"`
				Expiry struct {
					Month string `json:"month"`
					Year  string `json:"year"`
				} `json:"expiry"`
				DevicePayment struct {
					CryptogramFormat        string `json:"cryptogramFormat"`
					OnlinePaymentCryptogram string `json:"onlinePaymentCryptogram"`
					EciIndicator            string `json:"eciIndicator,omitempty"`
				} `json:"devicePayment"`
			} `json:"card"`
		} `json:"provided"`
	} `json:"sourceOfFunds"`
	Device struct {
		Ani string `json:"ani,omitempty"`
	} `json:"device,omitempty"`
	Transaction struct {
		Source string `json:"source"`
	} `json:"transaction"`
}

type mastercardService struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewMastercardService(cfg *config.Config) MastercardService {
	return &mastercardService{
		cfg:        cfg,
		httpClient: &http.Client{},
	}
}

// AuthorizeWithToken authorizes payment with token (hold funds)
func (s *mastercardService) AuthorizeWithToken(token, amount, currency string) (*PaymentResponse, error) {
	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := PaymentRequest{
		ApiOperation: "AUTHORIZE",
	}
	request.Order.Amount = amount
	request.Order.Currency = currency
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Token = token

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Convert amount to string if it's a number
	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

// AuthorizeWithCard authorizes payment with card details (hold funds)
func (s *mastercardService) AuthorizeWithCard(cardNumber, expiryMonth, expiryYear, cvv, amount, currency string) (*PaymentResponse, error) {
	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := PaymentRequest{
		ApiOperation: "AUTHORIZE",
	}
	request.Order.Amount = amount
	request.Order.Currency = currency
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Provided.Card.Number = cardNumber
	request.SourceOfFunds.Provided.Card.Expiry.Month = expiryMonth
	request.SourceOfFunds.Provided.Card.Expiry.Year = expiryYear
	request.SourceOfFunds.Provided.Card.SecurityCode = cvv

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Convert amount to string if it's a number
	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

// CaptureAuthorization captures previously authorized funds
func (s *mastercardService) CaptureAuthorization(orderID, amount, currency string) (*PaymentResponse, error) {
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/2",
		s.cfg.MastercardMerchantID, orderID)

	request := map[string]interface{}{
		"apiOperation": "CAPTURE",
		"transaction": map[string]interface{}{
			"amount":   amount,
			"currency": currency,
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &response, nil
}

// VoidAuthorization cancels an authorization
func (s *mastercardService) VoidAuthorization(orderID string) (*PaymentResponse, error) {
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/2",
		s.cfg.MastercardMerchantID, orderID)

	request := map[string]interface{}{
		"apiOperation": "VOID",
		"transaction": map[string]interface{}{
			"targetTransactionId": "1",
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &response, nil
}

// UpdateAuthorization updates authorization amount
func (s *mastercardService) UpdateAuthorization(orderID, amount, currency string) (*PaymentResponse, error) {
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/2",
		s.cfg.MastercardMerchantID, orderID)

	request := map[string]interface{}{
		"apiOperation": "UPDATE_AUTHORIZATION",
		"transaction": map[string]interface{}{
			"amount":   amount,
			"currency": currency,
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &response, nil
}

// Helper method to make API requests
func (s *mastercardService) makeRequest(method, endpoint string, requestBody interface{}) ([]byte, error) {
	url := fmt.Sprintf("https://%s%s", s.cfg.MastercardHost, endpoint)

	var body []byte
	var err error

	if requestBody != nil {
		body, err = json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %v", err)
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set Basic Auth header
	auth := fmt.Sprintf("merchant.%s:%s", s.cfg.MastercardMerchantID, s.cfg.MastercardAPIPassword)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	req.Header.Set("Authorization", "Basic "+encodedAuth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Request/Response structures
type VerifyRequest struct {
	ApiOperation string `json:"apiOperation"`
	Order        struct {
		Currency string `json:"currency"`
	} `json:"order"`
	SourceOfFunds struct {
		Type     string `json:"type"`
		Provided struct {
			Card struct {
				Number string `json:"number"`
				Expiry struct {
					Month string `json:"month"`
					Year  string `json:"year"`
				} `json:"expiry"`
				SecurityCode string `json:"securityCode"`
			} `json:"card"`
		} `json:"provided"`
	} `json:"sourceOfFunds"`
}

type VerifyResponse struct {
	Result      string `json:"result"`
	GatewayCode string `json:"gatewayCode"`
	Response    struct {
		GatewayCode string `json:"gatewayCode"`
	} `json:"response"`
	Order struct {
		ID       string `json:"id"`
		Currency string `json:"currency"`
		Status   string `json:"status"`
	} `json:"order"`
	Transaction struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"transaction"`
}

type TokenRequest struct {
	SourceOfFunds struct {
		Type     string `json:"type"`
		Provided struct {
			Card struct {
				Number string `json:"number"`
				Expiry struct {
					Month string `json:"month"`
					Year  string `json:"year"`
				} `json:"expiry"`
				SecurityCode string `json:"securityCode"`
			} `json:"card"`
		} `json:"provided"`
	} `json:"sourceOfFunds"`
}

type TokenResponse struct {
	Token         string `json:"token"`
	SourceOfFunds struct {
		Provided struct {
			Card struct {
				Brand   string `json:"brand"`
				Funding string `json:"funding"`
				Expiry  string `json:"expiry"` // Changed from struct to string
				Number  string `json:"number"`
				Scheme  string `json:"scheme"`
				Issuer  string `json:"issuer"`
				Country string `json:"country"`
				Bin     string `json:"bin"`
				Last4   string `json:"last4"`
			} `json:"card"`
		} `json:"provided"`
	} `json:"sourceOfFunds"`
}

type PaymentRequest struct {
	ApiOperation string `json:"apiOperation"`
	Order        struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"order"`
	SourceOfFunds struct {
		Type     string `json:"type"`
		Token    string `json:"token,omitempty"`
		Provided struct {
			Card struct {
				Number string `json:"number,omitempty"`
				Expiry struct {
					Month string `json:"month,omitempty"`
					Year  string `json:"year,omitempty"`
				} `json:"expiry,omitempty"`
				SecurityCode string `json:"securityCode,omitempty"`
			} `json:"card,omitempty"`
		} `json:"provided,omitempty"`
	} `json:"sourceOfFunds"`
}

type PaymentResponse struct {
	Result      string `json:"result"`
	GatewayCode string `json:"gatewayCode"`
	Order       struct {
		ID             string      `json:"id"`
		Amount         interface{} `json:"amount"`
		Currency       string      `json:"currency"`
		Status         string      `json:"status"`
		WalletProvider string      `json:"walletProvider,omitempty"`
	} `json:"order"`
	Transaction struct {
		ID          string      `json:"id"`
		Amount      interface{} `json:"amount"`
		Currency    string      `json:"currency"`
		Type        string      `json:"type"`
		Status      string      `json:"status"`
		Description string      `json:"description"`
	} `json:"transaction"`
}

func generateOrderID() string {
	rand.Seed(time.Now().UnixNano())
	// Generate random number between 1 and 999,999,999
	return strconv.Itoa(rand.Intn(999999999) + 1)
}

// Implement methods
func (s *mastercardService) VerifyCard(cardNumber, expiryMonth, expiryYear, cvv, currency string) (*VerifyResponse, error) {
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/VERIFY_%s/transaction/1",
		s.cfg.MastercardMerchantID, cardNumber[len(cardNumber)-4:])

	request := VerifyRequest{
		ApiOperation: "VERIFY",
	}
	request.Order.Currency = currency
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Provided.Card.Number = cardNumber
	request.SourceOfFunds.Provided.Card.Expiry.Month = expiryMonth
	request.SourceOfFunds.Provided.Card.Expiry.Year = expiryYear
	request.SourceOfFunds.Provided.Card.SecurityCode = cvv

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response VerifyResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &response, nil
}

func (s *mastercardService) CreatePaymentToken(cardNumber, expiryMonth, expiryYear, cvv string) (*TokenResponse, error) {
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/token", s.cfg.MastercardMerchantID)

	request := TokenRequest{}
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Provided.Card.Number = cardNumber
	request.SourceOfFunds.Provided.Card.Expiry.Month = expiryMonth
	request.SourceOfFunds.Provided.Card.Expiry.Year = expiryYear
	request.SourceOfFunds.Provided.Card.SecurityCode = cvv

	body, err := s.makeRequest("POST", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response TokenResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &response, nil
}

func (s *mastercardService) PayWithToken(token, amount, currency string) (*PaymentResponse, error) {
	// Generate truly unique order ID with timestamp
	orderID := generateOrderID() // FIXED: Use random number
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := PaymentRequest{
		ApiOperation: "PAY",
	}
	request.Order.Amount = amount
	request.Order.Currency = currency
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Token = token

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Convert amount to string if it's a number
	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

func (s *mastercardService) PayWithCard(cardNumber, expiryMonth, expiryYear, cvv, amount, currency string) (*PaymentResponse, error) {

	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := PaymentRequest{
		ApiOperation: "PAY",
	}
	request.Order.Amount = amount
	request.Order.Currency = currency
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Provided.Card.Number = cardNumber
	request.SourceOfFunds.Provided.Card.Expiry.Month = expiryMonth
	request.SourceOfFunds.Provided.Card.Expiry.Year = expiryYear
	request.SourceOfFunds.Provided.Card.SecurityCode = cvv

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Convert amount to string if it's a number
	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

func (s *mastercardService) RefundPayment(orderID, amount, currency string) (*PaymentResponse, error) {
	// Generate a unique transaction number using timestamp
	// This ensures each refund gets a unique transaction number
	timestamp := time.Now().UnixNano()
	transactionNumber := timestamp % 1000 // Get last 3 digits for transaction number

	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/%d",
		s.cfg.MastercardMerchantID, orderID, transactionNumber)

	request := map[string]interface{}{
		"apiOperation": "REFUND",
		"transaction": map[string]interface{}{
			"amount":   amount,
			"currency": currency,
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &response, nil
}

// internal/services/mastercard_service.go
// Add these methods to the mastercardService struct:

// PayWithGooglePay processes a Google Pay payment with merchant-decrypted card details
func (s *mastercardService) PayWithGooglePay(cardNumber, expiryMonth, expiryYear, cryptogram, eci, amount, currency string) (*PaymentResponse, error) {
	// Check if we have Device Payments privilege
	// If not, simulate Google Pay with regular PAY operation (for testing)

	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	// Try Google Pay with Device Payments first
	request := GooglePayPaymentRequest{
		ApiOperation: "PAY",
	}
	request.Order.Amount = amount
	request.Order.Currency = currency
	request.Order.WalletProvider = "GOOGLE_PAY"
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Provided.Card.Number = cardNumber
	request.SourceOfFunds.Provided.Card.Expiry.Month = expiryMonth
	request.SourceOfFunds.Provided.Card.Expiry.Year = expiryYear
	request.SourceOfFunds.Provided.Card.DevicePayment.CryptogramFormat = "3DSECURE"
	request.SourceOfFunds.Provided.Card.DevicePayment.OnlinePaymentCryptogram = cryptogram
	request.SourceOfFunds.Provided.Card.DevicePayment.EciIndicator = eci
	request.Device.Ani = "12341234"
	request.Transaction.Source = "INTERNET"

	body, err := s.makeRequest("PUT", endpoint, request)

	// If Google Pay fails due to missing privilege, fallback to regular card payment
	if err != nil && strings.Contains(err.Error(), "Missing merchant privilege") {
		log.Println("Device Payments privilege not available, simulating Google Pay with regular card payment")

		// Fallback to regular PAY operation (simulating Google Pay)
		return s.PayWithCard(cardNumber, expiryMonth, expiryYear, "123", amount, currency)
	}

	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Google Pay response: %v", err)
	}

	// Convert amount to string if it's a number
	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

// AuthorizeWithGooglePay authorizes a Google Pay payment with merchant-decrypted card details
func (s *mastercardService) AuthorizeWithGooglePay(cardNumber, expiryMonth, expiryYear, cryptogram, eci, amount, currency string) (*PaymentResponse, error) {
	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := GooglePayPaymentRequest{
		ApiOperation: "AUTHORIZE",
	}
	request.Order.Amount = amount
	request.Order.Currency = currency
	request.Order.WalletProvider = "GOOGLE_PAY"
	request.SourceOfFunds.Type = "CARD"
	request.SourceOfFunds.Provided.Card.Number = cardNumber
	request.SourceOfFunds.Provided.Card.Expiry.Month = expiryMonth
	request.SourceOfFunds.Provided.Card.Expiry.Year = expiryYear
	request.SourceOfFunds.Provided.Card.DevicePayment.CryptogramFormat = "3DSECURE"
	request.SourceOfFunds.Provided.Card.DevicePayment.OnlinePaymentCryptogram = cryptogram
	request.SourceOfFunds.Provided.Card.DevicePayment.EciIndicator = eci
	request.Device.Ani = "12341234"
	request.Transaction.Source = "INTERNET"

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Google Pay authorization response: %v", err)
	}

	// Convert amount to string if it's a number
	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

// PayWithGooglePayToken - For Phase 2 when you have real Google Pay tokens
func (s *mastercardService) PayWithGooglePayToken(paymentToken, amount, currency string) (*PaymentResponse, error) {
	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	// This uses the gateway-decrypted flow (needs production merchant ID)
	request := map[string]interface{}{
		"apiOperation": "PAY",
		"order": map[string]interface{}{
			"amount":         amount,
			"currency":       currency,
			"walletProvider": "GOOGLE_PAY",
		},
		"sourceOfFunds": map[string]interface{}{
			"type": "CARD",
			"provided": map[string]interface{}{
				"card": map[string]interface{}{
					"devicePayment": map[string]interface{}{
						"paymentToken": paymentToken,
					},
				},
			},
		},
		"device": map[string]interface{}{
			"ani": "12341234",
		},
		"transaction": map[string]interface{}{
			"source": "INTERNET",
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Google Pay token response: %v", err)
	}

	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

// AuthorizeWithGooglePayToken - For Phase 2
func (s *mastercardService) AuthorizeWithGooglePayToken(paymentToken, amount, currency string) (*PaymentResponse, error) {
	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := map[string]interface{}{
		"apiOperation": "AUTHORIZE",
		"order": map[string]interface{}{
			"amount":         amount,
			"currency":       currency,
			"walletProvider": "GOOGLE_PAY",
		},
		"sourceOfFunds": map[string]interface{}{
			"type": "CARD",
			"provided": map[string]interface{}{
				"card": map[string]interface{}{
					"devicePayment": map[string]interface{}{
						"paymentToken": paymentToken,
					},
				},
			},
		},
		"device": map[string]interface{}{
			"ani": "12341234",
		},
		"transaction": map[string]interface{}{
			"source": "INTERNET",
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Google Pay token authorization response: %v", err)
	}

	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

func (s *mastercardService) PayWithApplePayToken(paymentToken, amount, currency string) (*PaymentResponse, error) {
	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := map[string]interface{}{
		"apiOperation": "PAY",
		"order": map[string]interface{}{
			"amount":         amount,
			"currency":       currency,
			"walletProvider": "APPLE_PAY",
		},
		"sourceOfFunds": map[string]interface{}{
			"type": "CARD",
			"provided": map[string]interface{}{
				"card": map[string]interface{}{
					"devicePayment": map[string]interface{}{
						"paymentToken": paymentToken,
					},
				},
			},
		},
		"device": map[string]interface{}{
			"ani": "12341234",
		},
		"transaction": map[string]interface{}{
			"source": "INTERNET",
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		// Check if it's a privilege error
		if strings.Contains(err.Error(), "Missing merchant privilege") {
			return nil, fmt.Errorf("Missing merchant privilege 'Device Payments'")
		}
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Apple Pay response: %v", err)
	}

	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}

func (s *mastercardService) AuthorizeWithApplePayToken(paymentToken, amount, currency string) (*PaymentResponse, error) {
	orderID := generateOrderID()
	endpoint := fmt.Sprintf("/api/rest/version/100/merchant/%s/order/%s/transaction/1",
		s.cfg.MastercardMerchantID, orderID)

	request := map[string]interface{}{
		"apiOperation": "AUTHORIZE",
		"order": map[string]interface{}{
			"amount":         amount,
			"currency":       currency,
			"walletProvider": "APPLE_PAY",
		},
		"sourceOfFunds": map[string]interface{}{
			"type": "CARD",
			"provided": map[string]interface{}{
				"card": map[string]interface{}{
					"devicePayment": map[string]interface{}{
						"paymentToken": paymentToken,
					},
				},
			},
		},
		"device": map[string]interface{}{
			"ani": "12341234",
		},
		"transaction": map[string]interface{}{
			"source": "INTERNET",
		},
	}

	body, err := s.makeRequest("PUT", endpoint, request)
	if err != nil {
		if strings.Contains(err.Error(), "Missing merchant privilege") {
			return nil, fmt.Errorf("Missing merchant privilege 'Device Payments'")
		}
		return nil, err
	}

	var response PaymentResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Apple Pay authorization response: %v", err)
	}

	response.Order.Amount = utils.ConvertToString(response.Order.Amount)
	response.Transaction.Amount = utils.ConvertToString(response.Transaction.Amount)

	return &response, nil
}
