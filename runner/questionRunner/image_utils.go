package questionRunner

import (
	"benchmark_runner/types"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

var ErrImageHashMismatch = errors.New("image SHA-256 hash mismatch")

// PreflightImages downloads, verifies, and preprocesses every benchmark image.
func PreflightImages(questions []types.Question) error {
	for _, question := range questions {
		if _, err := processImageURLs(question.QuestionImages, question.QuestionImageHashes); err != nil {
			return fmt.Errorf("question %s: %w", question.ID, err)
		}
		for _, answer := range question.Answers {
			if _, err := processImageURLs(answer.Images, answer.ImageHashes); err != nil {
				return fmt.Errorf("question %s answer %s: %w", question.ID, answer.ID, err)
			}
		}
	}
	return nil
}

// downloadImage acquires a released benchmark image from its public URL.
func downloadImage(imgURL string) ([]byte, error) {
	fmt.Printf("Downloading image: %s\n", imgURL)
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(imgURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image from %s: %w", imgURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image from %s: status %d", imgURL, resp.StatusCode)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data from %s: %w", imgURL, err)
	}

	return imageData, nil
}

// processImageURLs downloads, normalizes, and encodes all images in a URL slice
func processImageURLs(urls, expectedHashes []string) ([][]byte, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	if len(expectedHashes) == 0 {
		expectedHashes = make([]string, len(urls))
	}
	if len(urls) != len(expectedHashes) {
		return nil, fmt.Errorf("received %d image URLs but %d image hashes", len(urls), len(expectedHashes))
	}

	bytes := make([][]byte, len(urls))
	for i, url := range urls {
		imgBytes, err := downloadImage(url)
		if err != nil {
			return nil, fmt.Errorf("failed to process image %d (%s): %w", i+1, url, err)
		}
		if err := verifyImageHash(imgBytes, expectedHashes[i], url); err != nil {
			return nil, fmt.Errorf("image %d: %w", i+1, err)
		}
		imgBytesNormalized, err := normalizeImage(imgBytes, 1024, 90)
		if err != nil {
			return nil, fmt.Errorf("failed to normalize image %d (%s): %w", i+1, url, err)
		}
		bytes[i] = imgBytesNormalized
	}
	return bytes, nil
}

func verifyImageHash(imageData []byte, expectedHash, imageURL string) error {
	if expectedHash == "" {
		return nil
	}
	actualHash := fmt.Sprintf("%x", sha256.Sum256(imageData))
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("%w for %s: expected %s, got %s", ErrImageHashMismatch, imageURL, expectedHash, actualHash)
	}
	return nil
}

// normalizeImage takes image bytes, normalizes, and returns JPEG bytes
func normalizeImage(imageData []byte, maxDim int, jpegQuality int) ([]byte, error) {
	// Decode image from bytes
	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		// If it's a JPEG with unsupported subsampling, try to re-encode it first
		if format == "jpeg" {
			// Try to re-encode the original JPEG with standard settings to fix subsampling issues
			reencoded, reencodeErr := reencodeJPEG(imageData, jpegQuality)
			if reencodeErr == nil {
				// Retry decoding the re-encoded JPEG
				img, _, err = image.Decode(bytes.NewReader(reencoded))
				if err != nil {
					return nil, fmt.Errorf("failed to decode re-encoded JPEG: %w", err)
				}
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// Get original dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate new dimensions preserving aspect ratio
	var newWidth, newHeight int
	if width > maxDim || height > maxDim {
		if width > height {
			newWidth = maxDim
			newHeight = height * maxDim / width
		} else {
			newHeight = maxDim
			newWidth = width * maxDim / height
		}
	} else {
		newWidth = width
		newHeight = height
	}

	// Create new image with RGB (no alpha)
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// High-quality resize using Catmull-Rom interpolation
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)

	// Encode to JPEG in memory
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: jpegQuality})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// reencodeJPEG uses external tools to re-encode a JPEG with standard subsampling
// This helps fix JPEGs with unsupported chroma subsampling ratios
func reencodeJPEG(imageData []byte, quality int) ([]byte, error) {
	// Strategy: Convert through PNG (lossless) first to strip problematic metadata,
	// then convert back to JPEG with standard settings
	// This forces a complete re-encode with standard subsampling

	// Try using ImageMagick convert command
	// Convert JPEG -> PNG -> JPEG to ensure clean subsampling
	cmd := exec.Command("convert", "jpeg:-", "png:-")
	cmd.Stdin = bytes.NewReader(imageData)

	var pngBuf bytes.Buffer
	cmd.Stdout = &pngBuf

	if err := cmd.Run(); err == nil {
		// Now convert PNG back to JPEG with explicit subsampling
		jpegCmd := exec.Command("convert", "png:-", "-quality", fmt.Sprintf("%d", quality), "-sampling-factor", "4:2:0", "jpeg:-")
		jpegCmd.Stdin = &pngBuf

		var jpegBuf bytes.Buffer
		jpegCmd.Stdout = &jpegBuf

		if err := jpegCmd.Run(); err == nil {
			return jpegBuf.Bytes(), nil
		}
	}

	// Fallback: try using libjpeg via djpeg (which is more tolerant of subsampling issues)
	djpegCmd := exec.Command("djpeg", "-ppm")
	djpegCmd.Stdin = bytes.NewReader(imageData)

	var ppmBuf bytes.Buffer
	djpegCmd.Stdout = &ppmBuf

	if err := djpegCmd.Run(); err == nil {
		// Now encode back to JPEG with cjpeg and explicit subsampling
		cjpegCmd := exec.Command("cjpeg", "-quality", fmt.Sprintf("%d", quality), "-optimize", "-sample", "2x2,1x1,1x1")
		cjpegCmd.Stdin = &ppmBuf

		var jpegBuf bytes.Buffer
		cjpegCmd.Stdout = &jpegBuf

		if err := cjpegCmd.Run(); err == nil {
			return jpegBuf.Bytes(), nil
		}
	}

	return nil, fmt.Errorf("unable to re-encode JPEG: ImageMagick convert or libjpeg (djpeg/cjpeg) not available")
}
