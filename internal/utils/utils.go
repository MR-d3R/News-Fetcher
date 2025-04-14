package utils

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

func CreateRequest(url, method, payloadStr, apiKey string) ([]byte, error) {
	maxRetries := 5
	initialBackoff := 1.0 // начальная задержка в секундах
	backoffFactor := 2.0  // множитель для экспоненциального роста задержки

	var body []byte

	for attempt := 0; attempt < maxRetries; attempt++ {
		payload := []byte(payloadStr)
		if apiKey != "" {
			url += "&apiKey=" + apiKey
		}
		req, err := http.NewRequest(method, url, bytes.NewBuffer(payload))
		if err != nil {
			return []byte{}, err
		}

		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		res, err := client.Do(req)
		if err != nil {
			return []byte{}, err
		}

		body, err = io.ReadAll(res.Body)
		res.Body.Close()

		if err != nil {
			return []byte{}, err
		}

		if res.StatusCode == 429 || (res.StatusCode == 200 && strings.Contains(string(body), "Too Many Requests")) {
			waitTime := initialBackoff * math.Pow(backoffFactor, float64(attempt))
			time.Sleep(time.Duration(waitTime * float64(time.Second)))
			continue
		}

		return body, nil
	}

	// Если все попытки исчерпаны
	return body, fmt.Errorf("превышено максимальное количество попыток (%d) из-за ограничения запросов", maxRetries)
}
