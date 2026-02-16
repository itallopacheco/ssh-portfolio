// Package spotify fornece um cliente para a Spotify Web API.
// Implementa autenticação OAuth 2.0 com refresh token e
// endpoints para buscar música atual e histórico recente.
package spotify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// Client é o cliente HTTP para a Spotify Web API.
// Thread-safe através de mutex para acesso ao access token.
//
// Fluxo de autenticação:
//   1. App configurado com Client ID, Secret e Refresh Token
//   2. Antes de cada request, verifica se access token é válido
//   3. Se expirado, usa refresh token para obter novo access token
//   4. Access token é usado no header Authorization: Bearer
type Client struct {
	clientID     string         // ID do app no Spotify Developer Dashboard
	clientSecret string         // Secret do app
	refreshToken string         // Token permanente para renovar access token
	accessToken  string         // Token temporário (~1h) para chamadas à API
	tokenExpiry  time.Time      // Quando o access token expira
	mu           sync.RWMutex   // Protege accessToken e tokenExpiry
	httpClient   *http.Client   // Cliente HTTP com timeout
}

// Track representa uma música do Spotify.
type Track struct {
	Name       string // Nome da música
	Artist     string // Nome do artista principal
	Album      string // Nome do álbum
	ArtworkURL string // URL da capa do álbum (640x640)
	IsPlaying  bool   // true se está tocando agora
}

// tokenResponse é a resposta do endpoint /api/token.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // Segundos até expirar (~3600)
}

// currentlyPlayingResponse é a resposta do endpoint /me/player/currently-playing.
type currentlyPlayingResponse struct {
	IsPlaying bool `json:"is_playing"`
	Item      *struct {
		Name  string `json:"name"`
		Album struct {
			Name   string `json:"name"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"album"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"item"`
}

// recentlyPlayedResponse é a resposta do endpoint /me/player/recently-played.
type recentlyPlayedResponse struct {
	Items []struct {
		Track struct {
			Name  string `json:"name"`
			Album struct {
				Name   string `json:"name"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"track"`
	} `json:"items"`
}

// NewClient cria um novo cliente Spotify.
// Parâmetros obtidos no Spotify Developer Dashboard + fluxo OAuth.
func NewClient(clientID, clientSecret, refreshToken string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// GetCurrentlyPlaying retorna a música tocando agora.
// Retorna nil se nada estiver tocando (status 204).
//
// Endpoint: GET /v1/me/player/currently-playing
// Scope necessário: user-read-currently-playing
func (c *Client) GetCurrentlyPlaying() (*Track, error) {
	log.Debug("Fetching currently playing track")

	if err := c.ensureValidToken(); err != nil {
		log.Error("Failed to get valid token", "error", err)
		return nil, fmt.Errorf("failed to get valid token: %w", err)
	}

	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/currently-playing", nil)
	if err != nil {
		log.Error("Failed to create request", "error", err)
		return nil, err
	}

	c.mu.RLock()
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	c.mu.RUnlock()

	log.Debug("Sending request to Spotify API", "url", req.URL.String())
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error("Request failed", "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	log.Debug("Received response", "status", resp.StatusCode)

	if resp.StatusCode == http.StatusNoContent {
		log.Debug("No content - nothing playing")
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("Spotify API error", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("spotify API error: %d", resp.StatusCode)
	}

	var data currentlyPlayingResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Error("Failed to decode response", "error", err)
		return nil, err
	}

	if data.Item == nil {
		log.Debug("No item in response")
		return nil, nil
	}

	track := &Track{
		Name:      data.Item.Name,
		Album:     data.Item.Album.Name,
		IsPlaying: data.IsPlaying,
	}

	if len(data.Item.Artists) > 0 {
		track.Artist = data.Item.Artists[0].Name
	}

	if len(data.Item.Album.Images) > 0 {
		track.ArtworkURL = data.Item.Album.Images[0].URL
	}

	log.Info("Got currently playing", "track", track.Name, "artist", track.Artist, "playing", track.IsPlaying)
	return track, nil
}

// GetRecentlyPlayed retorna a última música tocada.
// Útil como fallback quando nada está tocando.
//
// Endpoint: GET /v1/me/player/recently-played?limit=1
// Scope necessário: user-read-recently-played
func (c *Client) GetRecentlyPlayed() (*Track, error) {
	log.Debug("Fetching recently played track")

	if err := c.ensureValidToken(); err != nil {
		log.Error("Failed to get valid token", "error", err)
		return nil, fmt.Errorf("failed to get valid token: %w", err)
	}

	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/recently-played?limit=1", nil)
	if err != nil {
		log.Error("Failed to create request", "error", err)
		return nil, err
	}

	c.mu.RLock()
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	c.mu.RUnlock()

	log.Debug("Sending request to Spotify API", "url", req.URL.String())
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error("Request failed", "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	log.Debug("Received response", "status", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("Spotify API error", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("spotify API error: %d", resp.StatusCode)
	}

	var data recentlyPlayedResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Error("Failed to decode response", "error", err)
		return nil, err
	}

	if len(data.Items) == 0 {
		log.Debug("No items in recently played")
		return nil, nil
	}

	item := data.Items[0].Track
	track := &Track{
		Name:      item.Name,
		Album:     item.Album.Name,
		IsPlaying: false,
	}

	if len(item.Artists) > 0 {
		track.Artist = item.Artists[0].Name
	}

	if len(item.Album.Images) > 0 {
		track.ArtworkURL = item.Album.Images[0].URL
	}

	log.Info("Got recently played", "track", track.Name, "artist", track.Artist)
	return track, nil
}

// ensureValidToken garante que temos um access token válido.
// Se expirado ou inexistente, chama refreshAccessToken().
func (c *Client) ensureValidToken() error {
	c.mu.RLock()
	valid := c.accessToken != "" && time.Now().Before(c.tokenExpiry)
	c.mu.RUnlock()

	if valid {
		return nil
	}

	return c.refreshAccessToken()
}

// refreshAccessToken obtém um novo access token usando o refresh token.
//
// Endpoint: POST /api/token
// Auth: Basic base64(client_id:client_secret)
// Body: grant_type=refresh_token&refresh_token=xxx
//
// O access token dura ~1 hora. Renovamos 60s antes de expirar.
func (c *Client) refreshAccessToken() error {
	log.Debug("Refreshing access token")

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		log.Error("Failed to create token request", "error", err)
		return err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error("Token request failed", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("Failed to refresh token", "status", resp.StatusCode, "body", string(body))
		return fmt.Errorf("failed to refresh token: %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		log.Error("Failed to decode token response", "error", err)
		return err
	}

	c.mu.Lock()
	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)
	c.mu.Unlock()

	log.Info("Access token refreshed", "expires_in", tokenResp.ExpiresIn)
	return nil
}
