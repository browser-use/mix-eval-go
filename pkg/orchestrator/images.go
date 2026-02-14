package orchestrator

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	_ "image/png" // Register PNG decoder
	"log"
	"os"
	"strings"
)

const (
	maxImageWidth  = 1024
	maxImageHeight = 768
	jpegQuality    = 70
)

// downsizeScreenshot resizes and compresses a screenshot image
func downsizeScreenshot(data []byte, maxWidth, maxHeight, quality int) ([]byte, error) {
	// Decode image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to decode screenshot: %v", err)
		return data, nil // Return original on error
	}

	// Get original dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate new dimensions maintaining aspect ratio
	if width > maxWidth || height > maxHeight {
		ratio := minFloat(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
		newWidth := int(float64(width) * ratio)
		newHeight := int(float64(height) * ratio)

		// Create new image
		resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

		// Simple nearest-neighbor resize (good enough for screenshots)
		for y := 0; y < newHeight; y++ {
			for x := 0; x < newWidth; x++ {
				srcX := int(float64(x) / ratio)
				srcY := int(float64(y) / ratio)
				resized.Set(x, y, img.At(srcX, srcY))
			}
		}
		img = resized
	}

	// Encode as JPEG with compression
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		log.Printf("Failed to encode JPEG: %v", err)
		return data, nil // Return original on error
	}

	return buf.Bytes(), nil
}

// collectImageURLs builds a list of base64 data URLs from file paths and base64 strings
func collectImageURLs(screenshotPaths, screenshotsB64 []string) []string {
	var imageURLs []string
	seen := make(map[string]bool)

	addURL := func(url string) {
		if url != "" && !seen[url] {
			imageURLs = append(imageURLs, url)
			seen[url] = true
		}
	}

	// Process file paths
	for _, path := range screenshotPaths {
		if path == "" {
			continue
		}

		// Handle data URLs
		if strings.HasPrefix(path, "data:image") {
			// Decode, downsize, re-encode
			parts := strings.SplitN(path, ",", 2)
			if len(parts) == 2 {
				rawBytes, err := base64.StdEncoding.DecodeString(parts[1])
				if err != nil {
					log.Printf("Failed to decode data URL screenshot: %v", err)
					addURL(path) // Fall back to original
					continue
				}
				downsized, _ := downsizeScreenshot(rawBytes, maxImageWidth, maxImageHeight, jpegQuality)
				b64 := base64.StdEncoding.EncodeToString(downsized)
				addURL("data:image/jpeg;base64," + b64)
			} else {
				addURL(path)
			}
			continue
		}

		// Handle HTTP URLs (can't downsize, pass through)
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			addURL(path)
			continue
		}

		// Handle file paths
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Failed to load screenshot from path %s: %v", path, err)
			continue
		}
		downsized, _ := downsizeScreenshot(data, maxImageWidth, maxImageHeight, jpegQuality)
		b64 := base64.StdEncoding.EncodeToString(downsized)
		addURL("data:image/jpeg;base64," + b64)
	}

	// Process base64 strings
	for _, b64 := range screenshotsB64 {
		if b64 == "" {
			continue
		}

		// Handle data URL format vs raw base64
		var b64Data string
		if strings.HasPrefix(b64, "data:image") {
			parts := strings.SplitN(b64, ",", 2)
			if len(parts) == 2 {
				b64Data = parts[1]
			} else {
				addURL(b64)
				continue
			}
		} else {
			b64Data = b64
		}

		// Decode, downsize, re-encode
		rawBytes, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			log.Printf("Failed to decode base64 screenshot: %v", err)
			// Fall back to original
			if strings.HasPrefix(b64, "data:image") {
				addURL(b64)
			} else {
				addURL("data:image/png;base64," + b64)
			}
			continue
		}

		downsized, _ := downsizeScreenshot(rawBytes, maxImageWidth, maxImageHeight, jpegQuality)
		encoded := base64.StdEncoding.EncodeToString(downsized)
		addURL("data:image/jpeg;base64," + encoded)
	}

	return imageURLs
}
