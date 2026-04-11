package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	BinaryName          = "portico"
	defaultAPIBaseURL   = "https://api.github.com"
	defaultAssetBaseURL = "https://github.com"
	repoOwner           = "aliefe04"
	repoName            = "portico"
)

type Result struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Path           string
}

type Updater struct {
	Client       *http.Client
	APIBaseURL   string
	AssetBaseURL string
}

func (u Updater) SelfUpdate(ctx context.Context, currentVersion, execPath, goos, goarch string) (Result, error) {
	currentVersion = NormalizeVersion(currentVersion)
	if currentVersion == "" || currentVersion == "dev" {
		return Result{}, fmt.Errorf("self-update is unavailable for development builds")
	}

	latest, err := LatestTag(ctx, u.httpClient(), u.apiBaseURL())
	if err != nil {
		return Result{}, err
	}

	result := Result{
		CurrentVersion: currentVersion,
		LatestVersion:  latest,
		Path:           execPath,
	}
	if latest == currentVersion {
		return result, nil
	}

	assetNames, err := ArchiveNames(latest, goos, goarch)
	if err != nil {
		return Result{}, err
	}

	checksums, err := download(ctx, u.httpClient(), ChecksumsURL(u.assetBaseURL(), latest))
	if err != nil {
		return Result{}, err
	}
	assetName, expectedSum, err := ResolveChecksumAsset(checksums, assetNames)
	if err != nil {
		return Result{}, err
	}

	archive, err := download(ctx, u.httpClient(), AssetURL(u.assetBaseURL(), latest, assetName))
	if err != nil {
		return Result{}, err
	}
	actualSum := sha256.Sum256(archive)
	if hex.EncodeToString(actualSum[:]) != expectedSum {
		return Result{}, fmt.Errorf("download checksum mismatch for %s", assetName)
	}

	binary, err := ExtractBinary(archive, BinaryName)
	if err != nil {
		return Result{}, err
	}

	if err := ReplaceExecutable(execPath, binary); err != nil {
		return Result{}, err
	}

	result.Updated = true
	return result, nil
}

func LatestTag(ctx context.Context, client *http.Client, apiBaseURL string) (string, error) {
	body, err := download(ctx, client, strings.TrimRight(apiBaseURL, "/")+"/repos/"+repoOwner+"/"+repoName+"/releases/latest")
	if err != nil {
		return "", err
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return "", fmt.Errorf("release response missing tag name")
	}
	return NormalizeVersion(release.TagName), nil
}

func ArchiveNames(version, goos, goarch string) ([]string, error) {
	switch goos {
	case "darwin", "linux":
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", goos)
	}

	switch goarch {
	case "amd64", "arm64":
	default:
		return nil, fmt.Errorf("unsupported architecture: %s", goarch)
	}

	trimmedVersion := strings.TrimPrefix(NormalizeVersion(version), "v")
	return []string{
		fmt.Sprintf("%s_%s_%s.tar.gz", BinaryName, goos, goarch),
		fmt.Sprintf("%s_%s_%s_%s.tar.gz", BinaryName, trimmedVersion, goos, goarch),
	}, nil
}

func AssetURL(assetBaseURL, version, assetName string) string {
	return strings.TrimRight(assetBaseURL, "/") + "/" + repoOwner + "/" + repoName + "/releases/download/" + NormalizeVersion(version) + "/" + assetName
}

func ChecksumsURL(assetBaseURL, version string) string {
	return strings.TrimRight(assetBaseURL, "/") + "/" + repoOwner + "/" + repoName + "/releases/download/" + NormalizeVersion(version) + "/checksums.txt"
}

func NormalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" {
		return version
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func ChecksumForAsset(data []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found", assetName)
}

func ResolveChecksumAsset(data []byte, assetNames []string) (string, string, error) {
	for _, assetName := range assetNames {
		sum, err := ChecksumForAsset(data, assetName)
		if err == nil {
			return assetName, sum, nil
		}
	}
	return "", "", fmt.Errorf("checksum for supported release assets not found")
}

func ExtractBinary(archive []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		return io.ReadAll(tr)
	}

	return nil, fmt.Errorf("binary %s not found in archive", binaryName)
}

func ReplaceExecutable(path string, binary []byte) error {
	resolvedPath := path
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		resolvedPath = realPath
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace non-regular file: %s", resolvedPath)
	}

	current, err := os.ReadFile(resolvedPath)
	if err != nil {
		return err
	}

	backupPath := fmt.Sprintf("%s.bak.%s", resolvedPath, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.WriteFile(backupPath, current, info.Mode().Perm()); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(resolvedPath), BinaryName+"-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(binary); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(info.Mode().Perm()); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, resolvedPath); err != nil {
		return err
	}
	return nil
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func (u Updater) httpClient() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return http.DefaultClient
}

func (u Updater) apiBaseURL() string {
	if u.APIBaseURL != "" {
		return u.APIBaseURL
	}
	return defaultAPIBaseURL
}

func (u Updater) assetBaseURL() string {
	if u.AssetBaseURL != "" {
		return u.AssetBaseURL
	}
	return defaultAssetBaseURL
}
