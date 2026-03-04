package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go_proxy/anthropic"
	"go_proxy/api"
	"go_proxy/google"
	"go_proxy/openai"
)

// --- Configuração ---

type Provider struct {
	ID   string
	Host string
	Keys []string
}

type Config struct {
	Host      string
	Port      int
	ResetSecs int
	Providers map[string]*Provider
	Fallback  []string
}

// --- Key Rotator ---

type KeyRotator struct {
	keys      []string
	resetSecs int
	exhausted map[int]time.Time
	current   int
	resetAt   time.Time
	mu        sync.Mutex
}

func NewKeyRotator(keys []string, resetSecs int) *KeyRotator {
	return &KeyRotator{
		keys:      keys,
		resetSecs: resetSecs,
		exhausted: make(map[int]time.Time),
		resetAt:   time.Now().Add(time.Duration(resetSecs) * time.Second),
	}
}

func (kr *KeyRotator) maybeReset() {
	if time.Now().After(kr.resetAt) {
		kr.exhausted = make(map[int]time.Time)
		kr.current = 0
		kr.resetAt = time.Now().Add(time.Duration(kr.resetSecs) * time.Second)
	}
}

func (kr *KeyRotator) CurrentKey() string {
	kr.mu.Lock()
	defer kr.mu.Unlock()
	kr.maybeReset()
	if len(kr.keys) == 0 {
		return ""
	}
	for i := 0; i < len(kr.keys); i++ {
		idx := (kr.current + i) % len(kr.keys)
		if _, ok := kr.exhausted[idx]; !ok {
			kr.current = idx
			return kr.keys[idx]
		}
	}
	return ""
}

func (kr *KeyRotator) MarkRateLimited() bool {
	kr.mu.Lock()
	defer kr.mu.Unlock()
	kr.exhausted[kr.current] = time.Now()
	for i := 1; i < len(kr.keys); i++ {
		idx := (kr.current + i) % len(kr.keys)
		if _, ok := kr.exhausted[idx]; !ok {
			kr.current = idx
			return true
		}
	}
	return false
}

// --- Globals ---

var (
	config   Config
	rotators map[string]*KeyRotator
)

func loadEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Printf("⚠ Config file not found: %s", filename)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func initConfig() {
	loadEnvFile("config.env")

	config = Config{
		Host:      getEnv("OLLAMA_PROXY_HOST", "0.0.0.0"),
		Port:      getEnvInt("OLLAMA_PROXY_PORT", 11436),
		ResetSecs: getEnvInt("OLLAMA_PROXY_RESET", 3600),
		Providers: make(map[string]*Provider),
		Fallback:  []string{"ollama", "openrouter", "google"},
	}

	if order := os.Getenv("PROVIDER_FALLBACK_ORDER"); order != "" {
		parts := strings.Split(order, ",")
		var cleanParts []string
		for _, p := range parts {
			cleanParts = append(cleanParts, strings.TrimSpace(strings.ToLower(p)))
		}
		config.Fallback = cleanParts
	}

	addProvider("ollama", "OLLAMA_API_KEYS", "OLLAMA_UPSTREAM", "ollama.com")
	addProvider("openrouter", "OPENROUTER_API_KEYS", "OPENROUTER_UPSTREAM", "openrouter.ai")
	addProvider("google", "GOOGLE_API_KEYS", "GOOGLE_UPSTREAM", "generativelanguage.googleapis.com")

	rotators = make(map[string]*KeyRotator)
	for id, p := range config.Providers {
		rotators[id] = NewKeyRotator(p.Keys, config.ResetSecs)
	}
}

func addProvider(id, keysEnv, hostEnv, defaultHost string) {
	keysRaw := os.Getenv(keysEnv)
	if keysRaw == "" {
		return
	}
	keys := strings.Split(keysRaw, ",")
	for i := range keys {
		keys[i] = strings.TrimSpace(keys[i])
	}
	config.Providers[id] = &Provider{
		ID:   id,
		Host: getEnv(hostEnv, defaultHost),
		Keys: keys,
	}
	log.Printf("✅ Provider '%s': %d keys | host=%s", id, len(keys), config.Providers[id].Host)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var i int
	fmt.Sscanf(v, "%d", &i)
	return i
}

// --- Handlers ---

func detectProvider(model string) string {
	m := strings.ToLower(model)
	if strings.Contains(m, ":cloud") {
		return "ollama"
	}
	if strings.Contains(m, ":free") {
		return "openrouter"
	}
	if strings.Contains(m, "gemini") || strings.Contains(m, "palm") {
		return "google"
	}
	return ""
}

func cleanModelName(model string) string {
	parts := strings.Split(model, ":")
	var filtered []string
	for _, p := range parts {
		lp := strings.ToLower(p)
		if lp != "cloud" && lp != "free" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, ":")
}

func sendError(w http.ResponseWriter, format string, code int, message string) {
	log.Printf("❌ Error [%d]: %s", code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if format == "anthropic" {
		json.NewEncoder(w).Encode(anthropic.NewError(code, message))
	} else {
		json.NewEncoder(w).Encode(openai.NewError(code, message))
	}
}

func handleAnthropic(w http.ResponseWriter, r *http.Request) {
	reqID := time.Now().Format("150405")
	var req anthropic.MessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "anthropic", http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	providerID := detectProvider(req.Model)
	originalModel := req.Model
	req.Model = cleanModelName(req.Model)

	log.Printf("[%s] Anthropic Request: %s (Detected: %s)", reqID, originalModel, providerID)

	pIDs := config.Fallback
	if providerID != "" {
		pIDs = []string{providerID}
	}

	for _, pid := range pIDs {
		rotator := rotators[pid]
		if rotator == nil {
			continue
		}

		for {
			key := rotator.CurrentKey()
			if key == "" {
				log.Printf("[%s] ⚠ No keys available for provider: %s", reqID, pid)
				break
			}

			log.Printf("[%s] Trying provider: %s (Key: ...%s)", reqID, pid, key[len(key)-4:])
			ollamaReq, _ := anthropic.FromMessagesRequest(req)
			success, retry := callUpstream(pid, key, ollamaReq, w, "anthropic", reqID)
			if success {
				return
			}
			if !retry {
				break
			}
			log.Printf("[%s] ⚠ Rate limited on %s, rotating key...", reqID, pid)
			rotator.MarkRateLimited()
		}
	}

	sendError(w, "anthropic", http.StatusTooManyRequests, "All providers and keys exhausted for this request.")
}

func handleOpenAI(w http.ResponseWriter, r *http.Request) {
	reqID := time.Now().Format("150405")
	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "openai", http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	providerID := detectProvider(req.Model)
	originalModel := req.Model
	req.Model = cleanModelName(req.Model)

	log.Printf("[%s] OpenAI Request: %s (Detected: %s)", reqID, originalModel, providerID)

	pIDs := config.Fallback
	if providerID != "" {
		pIDs = []string{providerID}
	}

	for _, pid := range pIDs {
		rotator := rotators[pid]
		if rotator == nil {
			continue
		}

		for {
			key := rotator.CurrentKey()
			if key == "" {
				log.Printf("[%s] ⚠ No keys available for provider: %s", reqID, pid)
				break
			}

			log.Printf("[%s] Trying provider: %s (Key: ...%s)", reqID, pid, key[len(key)-4:])
			if pid == "google" {
				googleReq := google.FromOpenAIRequest(req)
				success, retry := callGoogleUpstream(pid, key, googleReq, w, req.Model, reqID)
				if success {
					return
				}
				if !retry {
					break
				}
			} else {
				ollamaReq, err := openai.FromChatRequest(req)
				if err != nil {
					log.Printf("[%s] ❌ Conversion Error: %v", reqID, err)
					break
				}

				success, retry := callUpstream(pid, key, ollamaReq, w, "openai", reqID)
				if success {
					return
				}
				if !retry {
					break
				}
			}
			log.Printf("[%s] ⚠ Rate limited on %s, rotating key...", reqID, pid)
			rotator.MarkRateLimited()
		}
	}
	sendError(w, "openai", http.StatusTooManyRequests, "All providers and keys exhausted for this request.")
}

func callGoogleUpstream(pid, key string, googleReq google.GooglePayload, w http.ResponseWriter, model string, reqID string) (success bool, retry bool) {
	provider := config.Providers[pid]
	url := fmt.Sprintf("https://%s/v1beta/models/%s:generateContent?key=%s", provider.Host, model, key)
	
	body, _ := json.Marshal(googleReq)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[%s] ❌ Upstream Error (%s): %v", reqID, pid, err)
		return false, false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return false, true
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[%s] ❌ Upstream Error (%s) Status %d: %s", reqID, pid, resp.StatusCode, string(respBody))
		return false, false
	}

	var googleResp google.GoogleResponse
	if err := json.NewDecoder(resp.Body).Decode(&googleResp); err != nil {
		log.Printf("[%s] ❌ JSON Decode Error (%s): %v", reqID, pid, err)
		return false, false
	}

	finalResp := google.ToOpenAIResponse(googleResp, model)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(finalResp)
	log.Printf("[%s] ✅ Success (%s)", reqID, pid)
	return true, false
}

func callUpstream(pid, key string, ollamaReq *api.ChatRequest, w http.ResponseWriter, format string, reqID string) (success bool, retry bool) {
	provider := config.Providers[pid]
	
	// Define a URL correta baseada no provedor
	url := fmt.Sprintf("https://%s/api/chat", provider.Host)
	if pid == "openrouter" {
		url = fmt.Sprintf("https://%s/api/v1/chat/completions", provider.Host)
	}
	
	// Se for OpenRouter ou outro compatível com OpenAI, precisamos converter o OllamaReq de volta para OpenAI
	var body []byte
	if pid == "openrouter" {
		// Converte de volta para o formato OpenAI que o OpenRouter espera
		openAIReq := openai.ChatCompletionRequest{
			Model: ollamaReq.Model,
			Stream: *ollamaReq.Stream,
		}
		if temp, ok := ollamaReq.Options["temperature"].(*float64); ok {
			openAIReq.Temperature = temp
		}
		for _, m := range ollamaReq.Messages {
			openAIReq.Messages = append(openAIReq.Messages, openai.Message{
				Role: m.Role,
				Content: m.Content,
			})
		}
		body, _ = json.Marshal(openAIReq)
	} else {
		body, _ = json.Marshal(ollamaReq)
	}

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	if pid == "openrouter" {
		req.Header.Set("HTTP-Referer", "https://github.com/ollama/ollama") // Opcional para OpenRouter
		req.Header.Set("X-Title", "Ollama Proxy Go")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[%s] ❌ Upstream Error (%s): %v", reqID, pid, err)
		return false, false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return false, true
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[%s] ❌ Upstream Error (%s) Status %d: %s", reqID, pid, resp.StatusCode, string(respBody))
		return false, false
	}

	if ollamaReq.Stream != nil && *ollamaReq.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		io.Copy(w, resp.Body)
		log.Printf("[%s] ✅ Success Stream (%s)", reqID, pid)
		return true, false
	}

	respBody, _ := io.ReadAll(resp.Body)
	
	// Se for OpenRouter, a resposta já vem no formato OpenAI
	if pid == "openrouter" {
		var openAIResp openai.ChatCompletion
		if err := json.Unmarshal(respBody, &openAIResp); err != nil {
			log.Printf("[%s] ❌ JSON Decode Error (%s): %v | Body: %s", reqID, pid, err, string(respBody))
			return false, false
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
		log.Printf("[%s] ✅ Success (%s)", reqID, pid)
		return true, false
	}

	var ollamaResp api.ChatResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		log.Printf("[%s] ❌ JSON Decode Error (%s): %v | Body: %s", reqID, pid, err, string(respBody))
		return false, false
	}

	var finalResp any
	if format == "anthropic" {
		finalResp = anthropic.ToMessagesResponse(anthropic.GenerateMessageID(), ollamaResp)
	} else {
		finalResp = openai.ToChatCompletion("chatcmpl-"+anthropic.GenerateMessageID(), ollamaResp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(finalResp)
	log.Printf("[%s] ✅ Success (%s)", reqID, pid)
	return true, false
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	initConfig()

	http.HandleFunc("/v1/messages", handleAnthropic)
	http.HandleFunc("/v1/chat/completions", handleOpenAI)

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	log.Printf("🚀 Proxy Go rodando em %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
