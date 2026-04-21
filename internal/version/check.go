// Package version checks for newer mneme releases on GitHub.
package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	repoOwner = "Edcko"
	repoName  = "Mneme"
)

var (
	checkTimeout           = 2 * time.Second
	githubLatestReleaseURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	githubTagsURL          = fmt.Sprintf("https://api.github.com/repos/%s/%s/tags", repoOwner, repoName)
	httpClient             = http.DefaultClient
)

type CheckStatus string

const (
	StatusUpToDate        CheckStatus = "up_to_date"
	StatusUpdateAvailable CheckStatus = "update_available"
	StatusCheckFailed     CheckStatus = "check_failed"
)

type CheckResult struct {
	Status  CheckStatus
	Message string
}

// githubRelease is the subset of the GitHub releases API we care about.
type githubRelease struct {
	TagName string `json:"tag_name"`
}

// githubTag is a single tag from the GitHub tags API.
type githubTag struct {
	Name string `json:"name"`
}

// CheckLatest compares the running version against the latest GitHub release.
// If the releases endpoint returns 404 (no published release), it falls back
// to fetching tags and picking the highest semver tag.
// It distinguishes between up-to-date, update available, and check failures.
func CheckLatest(current string) CheckResult {
	switch current {
	case "":
		return checkFailed("Could not check for updates: current version is unknown.")
	case "dev":
		return checkFailed("Could not check for updates: development builds do not map to a release version.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return checkFailed("Could not check for updates: could not create the GitHub request.")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return checkFailed("Could not check for updates: GitHub took too long to respond.")
		}
		return checkFailed(fmt.Sprintf("Could not check for updates: %v.", err))
	}
	defer resp.Body.Close()

	// 404 means no published release — fall back to tags from the same repo.
	if resp.StatusCode == http.StatusNotFound {
		latest, ferr := fetchLatestFromTags()
		if ferr != nil {
			return checkFailed(fmt.Sprintf("Could not check for updates: no releases found and tag lookup failed (%v).", ferr))
		}
		return compareWithLatest(current, latest)
	}

	if resp.StatusCode != http.StatusOK {
		return checkFailed(nonOKStatusMessage(resp.Status))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return checkFailed("Could not check for updates: could not read the GitHub response.")
	}

	latest := normalizeVersion(release.TagName)
	if latest == "" {
		return checkFailed("Could not check for updates: GitHub did not return a release version.")
	}

	return compareWithLatest(current, latest)
}

// compareWithLatest compares the running version against a resolved latest version.
func compareWithLatest(current, latest string) CheckResult {
	running := normalizeVersion(current)

	if latest == running {
		return CheckResult{Status: StatusUpToDate}
	}

	if !isNewer(latest, running) {
		return CheckResult{Status: StatusUpToDate}
	}

	return CheckResult{
		Status: StatusUpdateAvailable,
		Message: fmt.Sprintf(
			"Update available: %s -> %s\nTo update:\n%s",
			running, latest, updateInstructions(),
		),
	}
}

// fetchLatestFromTags queries the GitHub tags endpoint and returns the
// highest valid semver tag name (normalized, without "v" prefix).
func fetchLatestFromTags() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubTagsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tags endpoint returned %s", resp.Status)
	}

	var tags []githubTag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return "", fmt.Errorf("could not decode tags: %w", err)
	}

	var best string
	for _, tag := range tags {
		v := normalizeVersion(tag.Name)
		if !isSemver(v) {
			continue
		}
		if best == "" || isNewer(v, best) {
			best = v
		}
	}

	if best == "" {
		return "", errors.New("no valid semver tags found")
	}

	return best, nil
}

// isSemver returns true if v looks like a valid semver string (e.g. "1.2.3").
// It requires the string to start with a digit and contain at least one dot.
func isSemver(v string) bool {
	if v == "" || v[0] < '0' || v[0] > '9' {
		return false
	}
	return strings.Contains(v, ".")
}

// normalizeVersion strips a leading "v" prefix.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// isNewer returns true if latest > current using simple semver comparison.
func isNewer(latest, current string) bool {
	latestParts := splitVersion(latest)
	currentParts := splitVersion(current)

	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}
	return false
}

// splitVersion splits "1.8.1" into [1, 8, 1]. Returns [0,0,0] on parse failure.
func splitVersion(v string) [3]int {
	var parts [3]int
	segments := strings.SplitN(v, ".", 3)
	for i, s := range segments {
		if i >= 3 {
			break
		}
		for _, c := range s {
			if c >= '0' && c <= '9' {
				parts[i] = parts[i]*10 + int(c-'0')
			} else {
				break
			}
		}
	}
	return parts
}

// updateInstructions returns platform-appropriate update commands.
func updateInstructions() string {
	switch runtime.GOOS {
	case "darwin":
		return "  brew update && brew upgrade mneme"
	case "linux":
		return "  brew update && brew upgrade mneme\n  or: go install github.com/Edcko/Mneme/cmd/engram@latest"
	default:
		return "  go install github.com/Edcko/Mneme/cmd/engram@latest\n  or: https://github.com/Edcko/Mneme/releases/latest"
	}
}

func githubToken() string {
	if token := strings.TrimSpace(os.Getenv("GH_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
}

func nonOKStatusMessage(status string) string {
	msg := fmt.Sprintf("Could not check for updates: GitHub API returned %s.", status)
	if strings.HasPrefix(status, "401") || strings.HasPrefix(status, "403") {
		msg += " Set GH_TOKEN or GITHUB_TOKEN to reduce rate limits."
	}
	return msg
}

func checkFailed(message string) CheckResult {
	return CheckResult{Status: StatusCheckFailed, Message: message}
}
