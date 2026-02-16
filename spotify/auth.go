//go:build ignore

// Este arquivo é um helper para obter o refresh token do Spotify.
// Rode com: go run spotify/auth.go
//
// 1. Defina as variáveis de ambiente SPOTIFY_CLIENT_ID e SPOTIFY_CLIENT_SECRET
// 2. Rode este script
// 3. Acesse http://localhost:8888/login no browser
// 4. Autorize o app no Spotify
// 5. Copie o refresh token que será exibido no terminal

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var (
	clientID     = os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")
	redirectURI  = "http://127.0.0.1:8888/callback"
	scopes       = "user-read-currently-playing user-read-recently-played"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func main() {
	if clientID == "" || clientSecret == "" {
		log.Fatal("Defina SPOTIFY_CLIENT_ID e SPOTIFY_CLIENT_SECRET")
	}

	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/callback", handleCallback)

	fmt.Println("===========================================")
	fmt.Println("Spotify Auth Helper")
	fmt.Println("===========================================")
	fmt.Println("Acesse: http://localhost:8888/login")
	fmt.Println("===========================================")

	log.Fatal(http.ListenAndServe(":8888", nil))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", scopes)

	authURL := "https://accounts.spotify.com/authorize?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusFound)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Código não encontrado", http.StatusBadRequest)
		return
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("\n===========================================")
	fmt.Println("REFRESH TOKEN (copie este valor):")
	fmt.Println("===========================================")
	fmt.Println(tokenResp.RefreshToken)
	fmt.Println("===========================================")

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		<html>
		<body style="font-family: sans-serif; padding: 2rem;">
			<h1>✅ Sucesso!</h1>
			<p>Refresh token obtido. Verifique o terminal.</p>
			<p>Você pode fechar esta página.</p>
		</body>
		</html>
	`)
}
