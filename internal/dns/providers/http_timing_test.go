//go:build integration

package providers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestHTTPTimingPorkbun(t *testing.T) {
	client := &http.Client{Timeout: 30 * time.Second}
	body, _ := json.Marshal(map[string]string{"apikey": "invalid", "secretapikey": "invalid"})

	for i := range 3 {
		start := time.Now()
		resp, err := client.Post("https://api.porkbun.com/api/json/v3/domain/listAll", "application/json", bytes.NewReader(body))
		elapsed := time.Since(start)
		t.Logf("Call %d: %v (err=%v)", i+1, elapsed, err)
		if resp != nil {
			resp.Body.Close()
		}
	}
}
