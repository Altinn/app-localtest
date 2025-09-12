package fetcher

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const URL = "https://storage.googleapis.com/chrome-for-testing-public/117.0.5938.92/linux64/chrome-linux64.zip"

func Fetch(toPath string) (string, error) {
	zipPath := filepath.Join(toPath, "chrome-linux64.zip")
	outDir := toPath
	extractedDir := filepath.Join(outDir, "chrome-linux64")
	binPath := filepath.Join(extractedDir, "chrome")

	if _, err := os.Stat(binPath); err == nil {
		fmt.Printf("Browser already exists at %s\n", binPath)
		return binPath, nil
	}

	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		if err := os.MkdirAll(toPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create output directory: %v", err)
		}

		fmt.Println("Downloading Chrome...")
		resp, err := http.Get(URL)
		if err != nil {
			return "", fmt.Errorf("failed to download Chrome: %v", err)
		}
		defer resp.Body.Close()

		file, err := os.Create(zipPath)
		if err != nil {
			return "", fmt.Errorf("failed to create zip file: %v", err)
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to save Chrome zip: %v", err)
		}
		fmt.Printf("Browser downloaded to %s\n", zipPath)
	}

	if err := unzipFile(zipPath, outDir); err != nil {
		return "", fmt.Errorf("failed to extract Chrome: %v", err)
	}
	fmt.Printf("Browser extracted to %s\n", extractedDir)

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return "", fmt.Errorf("browser binary not found at %s", binPath)
	}

	return binPath, nil
}

func unzipFile(zipPath, outDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(outDir, sanitizeFilePath(file.Name))

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.FileInfo().Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()

		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			return err
		}
		defer targetFile.Close()

		_, err = io.Copy(targetFile, fileReader)
		if err != nil {
			return err
		}
	}

	return nil
}

func sanitizeFilePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")

	for i, part := range parts {
		if part == ".." || part == "." || strings.Contains(part, ":") || 
		   strings.HasPrefix(part, "~") || strings.ContainsAny(part, "<>:|?*") {
			parts[i] = "_"
		}
	}

	return filepath.Join(parts...)
}
