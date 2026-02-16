// Package albumart renderiza imagens como arte ASCII/Unicode no terminal.
// Usa caracteres de half-block (▀) com cores ANSI true color (24-bit)
// para criar uma representação visual de capas de álbum.
//
// Técnica: Cada caractere ▀ representa 2 pixels verticais.
// O pixel superior usa a cor de foreground, o inferior usa background.
// Isso dobra a resolução vertical efetiva.
package albumart

import (
	"fmt"
	"image"
	_ "image/jpeg" // Registra decoder JPEG
	_ "image/png"  // Registra decoder PNG
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

// Cache armazena imagens já renderizadas para evitar re-download.
// Usa LRU simples com TTL de 5 minutos e máximo de 10 entradas.
var (
	cache     = make(map[string]cacheEntry)
	cacheMu   sync.RWMutex
	cacheTTL  = 5 * time.Minute
	cacheSize = 10
)

// cacheEntry armazena uma imagem renderizada e quando foi criada.
type cacheEntry struct {
	rendered  string    // String com códigos ANSI já processados
	timestamp time.Time // Quando foi cacheado
}

// RenderFromURL baixa uma imagem e renderiza como blocos Unicode coloridos.
//
// Parâmetros:
//   - url: URL da imagem (JPEG ou PNG)
//   - width: largura em caracteres
//   - height: altura em linhas (cada linha = 2 pixels)
//
// Fluxo:
//   1. Verifica cache
//   2. Se não cacheado, baixa imagem via HTTP
//   3. Decodifica JPEG/PNG
//   4. Redimensiona para width × (height×2) pixels
//   5. Converte para string com códigos ANSI
//   6. Armazena no cache
//   7. Retorna string renderizada
func RenderFromURL(url string, width, height int) (string, error) {
	if url == "" {
		return renderPlaceholder(width, height), nil
	}

	// Check cache
	cacheMu.RLock()
	if entry, ok := cache[url]; ok {
		if time.Since(entry.timestamp) < cacheTTL {
			cacheMu.RUnlock()
			return entry.rendered, nil
		}
	}
	cacheMu.RUnlock()

	// Download image
	resp, err := http.Get(url)
	if err != nil {
		return renderPlaceholder(width, height), err
	}
	defer resp.Body.Close()

	// Decode image
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return renderPlaceholder(width, height), err
	}

	// Render to Unicode blocks
	rendered := renderImage(img, width, height)

	// Store in cache
	cacheMu.Lock()
	// Clean old entries if cache is full
	if len(cache) >= cacheSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range cache {
			if oldestKey == "" || v.timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.timestamp
			}
		}
		delete(cache, oldestKey)
	}
	cache[url] = cacheEntry{rendered: rendered, timestamp: time.Now()}
	cacheMu.Unlock()

	return rendered, nil
}

// renderImage converte uma imagem em blocos Unicode com cores true color.
//
// Formato ANSI true color (24-bit):
//   \x1b[38;2;R;G;Bm  → define cor de foreground (texto)
//   \x1b[48;2;R;G;Bm  → define cor de background
//   ▀                 → caractere half-block (metade superior)
//   \x1b[0m           → reset para cores padrão
//
// O caractere ▀ preenche a metade superior da célula.
// Combinando foreground (superior) e background (inferior),
// conseguimos 2 pixels por caractere.
func renderImage(img image.Image, width, height int) string {
	// Each character represents 2 vertical pixels
	// So we need width x (height*2) pixels
	pixelHeight := height * 2

	// Resize image
	resized := resizeImage(img, width, pixelHeight)

	var sb strings.Builder

	// Process 2 rows at a time (top pixel = foreground, bottom pixel = background)
	for y := 0; y < pixelHeight; y += 2 {
		for x := 0; x < width; x++ {
			// Top pixel (foreground)
			topR, topG, topB, _ := resized.At(x, y).RGBA()
			topR, topG, topB = topR>>8, topG>>8, topB>>8

			// Bottom pixel (background)
			var botR, botG, botB uint32
			if y+1 < pixelHeight {
				botR, botG, botB, _ = resized.At(x, y+1).RGBA()
				botR, botG, botB = botR>>8, botG>>8, botB>>8
			} else {
				botR, botG, botB = topR, topG, topB
			}

			// Write ANSI escape codes with upper half block
			// Foreground = top pixel, Background = bottom pixel
			sb.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				topR, topG, topB, botR, botG, botB))
		}
		sb.WriteString("\x1b[0m\n") // Reset and newline
	}

	result := sb.String()
	// Remove trailing newline
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result
}

// resizeImage redimensiona uma imagem para as dimensões especificadas.
// Usa interpolação Catmull-Rom para qualidade superior ao nearest-neighbor.
func resizeImage(img image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

// renderPlaceholder retorna um placeholder cinza quando não há imagem.
// Usado quando a URL está vazia ou o download falhou.
func renderPlaceholder(width, height int) string {
	var sb strings.Builder
	gray := "\x1b[38;2;60;60;60m\x1b[48;2;40;40;40m▀"

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sb.WriteString(gray)
		}
		sb.WriteString("\x1b[0m\n")
	}

	result := sb.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result
}

// ClearCache limpa o cache de imagens.
// Útil para liberar memória ou forçar re-download.
func ClearCache() {
	cacheMu.Lock()
	cache = make(map[string]cacheEntry)
	cacheMu.Unlock()
}
