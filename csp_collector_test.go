package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

var defaultViolationReportHandler = violationReportHandler{
	blockedURIs:                 defaultIgnoredBlockedURIs,
	truncateQueryStringFragment: false,
}

func TestHandlerForDisallowedMethods(t *testing.T) {
	disallowedMethods := []string{"GET", "DELETE", "PUT", "TRACE", "PATCH"}
	randomUrls := []string{"/", "/blah"}

	for _, method := range disallowedMethods {
		for _, url := range randomUrls {
			t.Run(method+url, func(t *testing.T) {
				request, err := http.NewRequest(method, url, nil)
				if err != nil {
					t.Fatalf("failed to create request: %v", err)
				}
				recorder := httptest.NewRecorder()
				defaultViolationReportHandler.ServeHTTP(recorder, request)

				response := recorder.Result()
				defer response.Body.Close()

				if response.StatusCode != http.StatusMethodNotAllowed {
					t.Errorf("expected HTTP status %v; got %v", http.StatusMethodNotAllowed, response.StatusCode)
				}
			})
		}
	}
}

func TestHandlerWithMetadata(t *testing.T) {
	csp := CSPReport{
		CSPReportBody{
			DocumentURI: "http://example.com",
			BlockedURI:  "http://example.com",
		},
	}

	payload, _ := json.Marshal(csp)

	for _, repeats := range []int{1, 2} {
		var logBuffer bytes.Buffer
		log.SetOutput(&logBuffer)

		url := "/?"
		for i := 0; i < repeats; i++ {
			url += fmt.Sprintf("metadata=value%d&", i)
		}

		request, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		recorder := httptest.NewRecorder()

		defaultViolationReportHandler.ServeHTTP(recorder, request)

		response := recorder.Result()
		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {
			t.Errorf("expected HTTP status %v; got %v", http.StatusOK, response.StatusCode)
		}

		log := logBuffer.String()
		if !strings.Contains(log, "metadata=value0") {
			t.Fatalf("Logged result should contain metadata value0 in '%s'", log)
		}
		if strings.Contains(log, "metadata=value1") {
			t.Fatalf("Logged result shouldn't contain metadata value1 in '%s'", log)
		}
	}
}

func TestHandlerWithMetadataObject(t *testing.T) {
	csp := CSPReport{
		CSPReportBody{
			DocumentURI: "http://example.com",
			BlockedURI:  "http://example.com",
		},
	}

	payload, _ := json.Marshal(csp)

	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)

	request, err := http.NewRequest("POST", "/path?a=b&c=d", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	recorder := httptest.NewRecorder()

	objectHandler := defaultViolationReportHandler
	objectHandler.metadataObject = true
	objectHandler.ServeHTTP(recorder, request)

	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Errorf("expected HTTP status %v; got %v", http.StatusOK, response.StatusCode)
	}

	log := logBuffer.String()
	if !strings.Contains(log, "metadata=\"map[a:b c:d]\"") {
		t.Fatalf("Logged result should contain metadata map '%s'", log)
	}
}

func TestValidateViolationWithInvalidBlockedURIs(t *testing.T) {
	invalidBlockedURIs := []string{
		"resource://",
		"chromenull://",
		"chrome-extension://",
		"safari-extension://",
		"mxjscall://",
		"webviewprogressproxy://",
		"res://",
		"mx://",
		"safari-resource://",
		"chromeinvoke://",
		"chromeinvokeimmediate://",
		"mbinit://",
		"opera://",
		"localhost",
		"127.0.0.1",
		"none://",
		"about:blank",
		"android-webview",
		"ms-browser-extension",
		"wvjbscheme://__wvjb_queue_message__",
		"nativebaiduhd://adblock",
		"bdvideo://error",
	}

	for _, blockedURI := range invalidBlockedURIs {
		// Makes the test name more readable for the output.
		testName := strings.Replace(blockedURI, "://", "", -1)

		t.Run(testName, func(t *testing.T) {
			rawReport := []byte(fmt.Sprintf(`{
				"csp-report": {
					"document-uri": "https://example.com",
					"blocked-uri": "%s"
				}
			}`, blockedURI))

			var report CSPReport
			jsonErr := json.Unmarshal(rawReport, &report)
			if jsonErr != nil {
				fmt.Println("error:", jsonErr)
			}

			validateErr := defaultViolationReportHandler.validateViolation(report)
			if validateErr == nil {
				t.Errorf("expected error to be raised but it didn't")
			}

			if validateErr.Error() != fmt.Sprintf("blocked URI ('%s') is an invalid resource", blockedURI) {
				t.Errorf("expected error to include correct message string but it didn't")
			}
		})
	}
}

func TestValidateViolationWithValidBlockedURIs(t *testing.T) {
	rawReport := []byte(`{
		"csp-report": {
			"document-uri": "https://example.com",
			"blocked-uri": "https://google.com/example.css"
		}
	}`)

	var report CSPReport
	jsonErr := json.Unmarshal(rawReport, &report)
	if jsonErr != nil {
		fmt.Println("error:", jsonErr)
	}

	validateErr := defaultViolationReportHandler.validateViolation(report)
	if validateErr != nil {
		t.Errorf("expected error not be raised")
	}
}

func TestValidateNonHttpDocumentURI(t *testing.T) {
	log.SetOutput(io.Discard)

	report := CSPReport{Body: CSPReportBody{
		BlockedURI:  "http://example.com/",
		DocumentURI: "about",
	}}

	validateErr := defaultViolationReportHandler.validateViolation(report)
	if validateErr.Error() != "document URI ('about') is invalid" {
		t.Errorf("expected error to include correct message string but it didn't")
	}
}

func TestHandleViolationReportMultipleTypeStatusCode(t *testing.T) {
	// Discard the output we create from the calls here.
	log.SetOutput(io.Discard)

	statusCodeValues := []interface{}{"200", 200}

	for _, statusCode := range statusCodeValues {
		t.Run(fmt.Sprintf("%T", statusCode), func(t *testing.T) {
			csp := CSPReport{
				CSPReportBody{
					DocumentURI: "https://example.com",
					StatusCode:  statusCode,
				},
			}

			payload, err := json.Marshal(csp)
			if err != nil {
				t.Fatalf("failed to marshal JSON: %v", err)
			}

			request, err := http.NewRequest("POST", "/", bytes.NewBuffer(payload))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			recorder := httptest.NewRecorder()
			defaultViolationReportHandler.ServeHTTP(recorder, request)

			response := recorder.Result()
			defer response.Body.Close()

			if response.StatusCode != http.StatusOK {
				t.Errorf("expected HTTP status %v; got %v", http.StatusOK, response.StatusCode)
			}
		})
	}
}

func TestFilterListProcessing(t *testing.T) {
	// Discard the output we create from the calls here.
	log.SetOutput(io.Discard)

	blockList := []string{
		"resource://",
		"",
		"# comment",
		"chrome-extension://",
		"",
	}

	trimmed := trimEmptyAndComments(blockList)

	if len(trimmed) != 2 {
		t.Errorf("expected filter list length 2; got %v", len(trimmed))
	}
	if trimmed[0] != "resource://" {
		t.Errorf("unexpected list entry; got %v", trimmed[0])
	}
	if trimmed[1] != "chrome-extension://" {
		t.Errorf("unexpected list entry; got %v", trimmed[1])
	}
}

func TestLogsPath(t *testing.T) {
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)

	csp := CSPReport{
		CSPReportBody{
			DocumentURI: "http://example.com",
			BlockedURI:  "http://example.com",
		},
	}

	payload, _ := json.Marshal(csp)

	url := "/deep/link"

	request, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	recorder := httptest.NewRecorder()

	defaultViolationReportHandler.ServeHTTP(recorder, request)

	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Errorf("expected HTTP status %v; got %v", http.StatusOK, response.StatusCode)
	}

	log := logBuffer.String()
	if !strings.Contains(log, "path=/deep/link") {
		t.Fatalf("Logged result should contain path value in '%s'", log)
	}
}

func TestTruncateQueryStringFragment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		original string
		expected string
	}{
		{"http://localhost.com/?test#anchor", "http://localhost.com/"},
		{"http://example.invalid", "http://example.invalid"},
		{"http://example.invalid#a", "http://example.invalid"},
		{"http://example.invalid?a", "http://example.invalid"},
		{"http://example.invalid#b?a", "http://example.invalid"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.original, func(t *testing.T) {
			t.Parallel()
			actual := truncateQueryStringFragment(tc.original)
			if actual != tc.expected {
				t.Errorf("truncating '%s' yielded '%s', expected '%s'", tc.original, actual, tc.expected)
			}
		})
	}
}
