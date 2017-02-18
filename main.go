package main

import (
	"encoding/json"
	"net/http"
)

type CSPReport struct {
	Body struct {
		DocumentURI        string `json:"document-uri"`
		Referrer           string `json:"referrer"`
		BlockedURI         string `json:"blocked-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		OriginalPolicy     string `json:"original-policy"`
	} `json:"csp-report"`
}

func main() {
	http.HandleFunc("/", handleViolationReport)
	http.ListenAndServe(":80", nil)
}
func handleViolationReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	decoder := json.NewDecoder(r.Body)
	var report CSPReport

	err := decoder.Decode(&report)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

}
