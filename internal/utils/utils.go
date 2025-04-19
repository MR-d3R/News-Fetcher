package utils

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

// CreateRequest sends an HTTP request with proper cookie handling and retries
func CreateRequest(urlStr, method, payloadStr, apiKey string) ([]byte, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return []byte{}, fmt.Errorf("failed to create cookie jar: %v", err)
	}

	client := &http.Client{
		Timeout: time.Second * 10,
		Jar:     jar,
	}

	maxRetries := 5
	initialBackoff := 1.0 // seconds
	backoffFactor := 2.0

	var body []byte

	for attempt := 0; attempt < maxRetries; attempt++ {
		finalURL := urlStr
		if apiKey != "" && !strings.Contains(urlStr, "apiKey") {

			if strings.Contains(urlStr, "?") {
				finalURL += "&apiKey=" + apiKey
			} else {
				finalURL += "?apiKey=" + apiKey
			}
		}

		payload := []byte(payloadStr)
		req, err := http.NewRequest(method, finalURL, bytes.NewBuffer(payload))
		if err != nil {
			return []byte{}, fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml,application/json;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "ru-RU,ru;q=0.8,en-US;q=0.5,en;q=0.3")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")

		if attempt > 0 {
			parsedURL, parseErr := url.Parse(finalURL)
			if parseErr == nil {
				baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
				req.Header.Set("Referer", baseURL)
			}
		}

		res, err := client.Do(req)
		if err != nil {
			return []byte{}, fmt.Errorf("request failed: %v", err)
		}

		body, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return []byte{}, fmt.Errorf("failed to read response body: %v", err)
		}

		if res.StatusCode == 429 || (res.StatusCode == 200 && strings.Contains(string(body), "Too Many Requests")) {
			waitTime := initialBackoff * math.Pow(backoffFactor, float64(attempt))
			time.Sleep(time.Duration(waitTime * float64(time.Second)))
			continue
		}

		if res.StatusCode != 200 {
			return body, fmt.Errorf("request failed with status code %d: %s", res.StatusCode, string(body))
		}

		return body, nil
	}

	return body, fmt.Errorf("exceeded maximum number of attempts (%d) due to request limitations", maxRetries)
}
