package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// BrowserProvider represents a cloud browser service
type BrowserProvider string

const (
	ProviderBrowserbase   BrowserProvider = "browserbase"
	ProviderBrightdata    BrowserProvider = "brightdata"
	ProviderHyperbrowser  BrowserProvider = "hyperbrowser"
	ProviderAnchorBrowser BrowserProvider = "anchor"
)

// BrowserSession contains CDP connection info
type BrowserSession struct {
	CDPURL    string
	SessionID string
	Provider  BrowserProvider
}

// CreateBrowserbaseSession creates a Browserbase browser session
func CreateBrowserbaseSession(apiKey, projectID string) (*BrowserSession, error) {
	reqBody := map[string]string{
		"projectId": projectID,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://www.browserbase.com/v1/sessions", bytes.NewBuffer(body))
	req.Header.Set("x-bb-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("browserbase API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("browserbase returned status %d", resp.StatusCode)
	}

	var result struct {
		ID         string `json:"id"`
		ConnectURL string `json:"connectUrl"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &BrowserSession{
		CDPURL:    result.ConnectURL,
		SessionID: result.ID,
		Provider:  ProviderBrowserbase,
	}, nil
}

// CreateBrightdataSession creates a Brightdata CDP session
func CreateBrightdataSession(username, password string) (*BrowserSession, error) {
	cdpURL := fmt.Sprintf("wss://%s:%s@brd.superproxy.io:9222", username, password)

	return &BrowserSession{
		CDPURL:    cdpURL,
		SessionID: "brightdata-session",
		Provider:  ProviderBrightdata,
	}, nil
}

// CreateHyperbrowserSession creates a Hyperbrowser session
func CreateHyperbrowserSession(apiKey string) (*BrowserSession, error) {
	reqBody := map[string]interface{}{
		"stealth": true,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://cloud.hyperbrowser.ai/v1/sessions", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hyperbrowser API error: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		SessionID string `json:"sessionId"`
		CdpUrl    string `json:"cdpUrl"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &BrowserSession{
		CDPURL:    result.CdpUrl,
		SessionID: result.SessionID,
		Provider:  ProviderHyperbrowser,
	}, nil
}

// CreateAnchorBrowserSession creates an Anchor Browser session
func CreateAnchorBrowserSession(apiKey string, mobile bool) (*BrowserSession, error) {
	reqBody := map[string]interface{}{
		"mobile":         mobile,
		"captchaSolving": true,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.anchorbrowser.io/browser", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anchor browser API error: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		SessionID string `json:"sessionId"`
		CdpUrl    string `json:"cdpUrl"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &BrowserSession{
		CDPURL:    result.CdpUrl,
		SessionID: result.SessionID,
		Provider:  ProviderAnchorBrowser,
	}, nil
}

// CloseBrowserSession closes a browser session
func CloseBrowserSession(session *BrowserSession, apiKey string) error {
	switch session.Provider {
	case ProviderBrowserbase:
		url := fmt.Sprintf("https://www.browserbase.com/v1/sessions/%s", session.SessionID)
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Set("x-bb-api-key", apiKey)
		_, err := http.DefaultClient.Do(req)
		return err
	case ProviderHyperbrowser:
		url := fmt.Sprintf("https://cloud.hyperbrowser.ai/v1/sessions/%s", session.SessionID)
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		_, err := http.DefaultClient.Do(req)
		return err
	default:
		return nil
	}
}
