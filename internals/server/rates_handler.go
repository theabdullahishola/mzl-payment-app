package server

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

type ExternalRateResponse struct {
    Result   string             `json:"result"`
    BaseCode string             `json:"base_code"`
    Rates    map[string]float64 `json:"rates"`
}

var (
    cachedRates     map[string]float64
    cacheExpiryTime time.Time
    ratesMutex      sync.RWMutex 
)

func (s *Server) GetExchangeRatesHandler(w http.ResponseWriter, r *http.Request) {
    ratesMutex.RLock()
    if cachedRates != nil && time.Now().Before(cacheExpiryTime) {
        currentRates := cachedRates
        ratesMutex.RUnlock()

        utils.JSON(w, r, http.StatusOK, map[string]interface{}{
            "status": "success",
            "data": map[string]interface{}{
                "base":  "USD",
                "rates": currentRates,
            },
        })
        return
    }
    ratesMutex.RUnlock()

    baseCurrency := "USD"
    url := fmt.Sprintf("https://open.er-api.com/v6/latest/%s", baseCurrency)


    req, err := http.NewRequestWithContext(r.Context(), "GET", url, nil)
    if err != nil {
        s.Logger.Error("failed to create request", "error", err)
        utils.ErrorJSON(w, r, http.StatusInternalServerError, err)
        return
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        s.Logger.Error("failed to fetch rates", "error", err)
        utils.ErrorJSON(w, r, http.StatusServiceUnavailable, err)
        return
    }
    defer resp.Body.Close()

    var apiData ExternalRateResponse
    if err := json.NewDecoder(resp.Body).Decode(&apiData); err != nil {
        s.Logger.Error("failed to decode rates", "error", err)
        utils.ErrorJSON(w, r, http.StatusInternalServerError, err)
        return
    }

  
    ratesMutex.Lock()
    cachedRates = apiData.Rates
    cacheExpiryTime = time.Now().Add(1 * time.Hour)
    ratesMutex.Unlock()

    utils.JSON(w, r, http.StatusOK, map[string]interface{}{
        "status": "success",
        "data": map[string]interface{}{
            "base":  baseCurrency, 
            "rates": apiData.Rates,
        },
    })
}