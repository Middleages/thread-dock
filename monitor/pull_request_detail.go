package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const pullRequestDetailFields = "number,url,state,reviewDecision,statusCheckRollup,closingIssuesReferences"

// canonicalPullRequestURL accepts the URL forms users can copy from GitHub,
// then returns one stable identity for matching and cache keys.
func canonicalPullRequestURL(raw, host string) (string, repositoryTarget, int, error) {
	host, err := normalizeGitHubHost(host)
	if err != nil {
		return "", repositoryTarget{}, 0, err
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || strings.ToLower(parsed.Host) != host || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", repositoryTarget{}, 0, errors.New("pull request URL must use the configured GitHub Host")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || (parts[2] != "pull" && parts[2] != "pulls") || !repoPattern.MatchString(parts[0]+"/"+parts[1]) {
		return "", repositoryTarget{}, 0, errors.New("pull request URL is malformed")
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", repositoryTarget{}, 0, errors.New("pull request number must be positive")
	}
	canonical := fmt.Sprintf("https://%s/%s/%s/pull/%d", host, parts[0], parts[1], number)
	return canonical, repositoryTarget{owner: parts[0], repo: parts[1]}, number, nil
}

func (m *GitHubMonitor) GetPullRequestDetail(ctx context.Context, rawURL string) (PullRequestDetail, error) {
	canonical, repo, number, err := canonicalPullRequestURL(rawURL, m.host)
	if err != nil {
		return PullRequestDetail{}, err
	}
	if err := ctx.Err(); err != nil {
		return PullRequestDetail{}, err
	}
	now := m.now()
	m.mu.Lock()
	cached, exists := m.cache[detailCacheKey(canonical)]
	if exists && now.Before(cached.nextAttemptAt) {
		if !cached.hasData {
			detail := PullRequestDetail{URL: canonical, Notice: cached.notice}
			m.mu.Unlock()
			return detail, nil
		}
		detail := cachedPullRequestDetail(cached.data, canonical, cached.stale)
		detail.Notice = cached.notice
		m.mu.Unlock()
		return detail, nil
	}
	if exists && cached.hasData && now.Sub(cached.observedAt) < githubTTL {
		detail := cachedPullRequestDetail(cached.data, canonical, false)
		m.mu.Unlock()
		return detail, nil
	}
	if m.detailInFlight == nil {
		m.detailInFlight = make(map[string]*pullRequestDetailCall)
	}
	call := m.detailInFlight[canonical]
	if call == nil {
		call = &pullRequestDetailCall{done: make(chan struct{})}
		m.detailInFlight[canonical] = call
		go m.completePullRequestDetail(call, canonical, repo, number, cached)
	}
	m.mu.Unlock()
	select {
	case <-call.done:
		return call.detail, call.err
	case <-ctx.Done():
		return PullRequestDetail{}, ctx.Err()
	}
}

func detailCacheKey(canonical string) string { return "pull-request-detail:" + canonical }

func (m *GitHubMonitor) completePullRequestDetail(call *pullRequestDetailCall, canonical string, repo repositoryTarget, number int, cached cachedSource) {
	call.detail, call.err = m.loadPullRequestDetail(canonical, repo, number, cached)
	m.mu.Lock()
	delete(m.detailInFlight, canonical)
	m.mu.Unlock()
	close(call.done)
}

func (m *GitHubMonitor) loadPullRequestDetail(canonical string, repo repositoryTarget, number int, cached cachedSource) (PullRequestDetail, error) {
	key := detailCacheKey(canonical)
	args := []string{"gh", "pr", "view", strconv.Itoa(number), "--repo", repo.owner + "/" + repo.repo, "--json", pullRequestDetailFields}
	output, err := m.command.run(context.Background(), args[0], args[1:]...)
	var raw rawPullRequest
	if err == nil {
		err = decodePullRequestDetail(output, &raw)
		if err == nil && (raw.Number == nil || *raw.Number != number) {
			err = errors.New("pull request detail number mismatch")
		}
		if err == nil {
			responseURL := canonicalItemURL(raw.URL, m.host)
			if responseURL == "" || locatorIdentity(responseURL, m.host) != locatorIdentity(canonical, m.host) {
				err = errors.New("pull request detail identity mismatch")
			}
		}
	}
	now := m.now()
	if err == nil {
		detail := mapPullRequestDetail(raw, canonical, now, false)
		m.mu.Lock()
		m.cache[key] = cachedSource{data: detail.GitHub, observedAt: now, nextAttemptAt: now.Add(githubTTL), hasData: true}
		m.mu.Unlock()
		return detail, nil
	}
	notice := pullRequestDetailNotice(err)
	cached.nextAttemptAt = now.Add(githubTTL)
	cached.stale = true
	cached.notice = notice
	m.mu.Lock()
	m.cache[key] = cached
	m.mu.Unlock()
	if cached.hasData {
		detail := cachedPullRequestDetail(cached.data, canonical, true)
		detail.Notice = notice
		return detail, nil
	}
	return PullRequestDetail{URL: canonical, Notice: notice}, nil
}

func decodePullRequestDetail(data []byte, raw *rawPullRequest) error {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil || shape == nil {
		return errors.New("invalid pull request detail shape")
	}
	if !requiredJSONNumber(shape, "number") || !requiredJSONString(shape, "url") || !requiredJSONString(shape, "state") {
		return errors.New("invalid pull request detail shape")
	}
	if err := optionalJSONText(shape, "reviewDecision"); err != nil {
		return err
	}
	if err := optionalJSONArrayObjects(shape, "statusCheckRollup"); err != nil {
		return err
	}
	if err := optionalJSONArrayObjects(shape, "closingIssuesReferences"); err != nil {
		return err
	}
	if err := json.Unmarshal(data, raw); err != nil {
		return errors.New("invalid pull request detail shape")
	}
	return nil
}

func mapPullRequestDetail(raw rawPullRequest, canonical string, observed time.Time, stale bool) PullRequestDetail {
	related := make([]string, 0, len(raw.ClosingIssuesReferences))
	for _, reference := range raw.ClosingIssuesReferences {
		if value := canonicalIssueURL(reference.URL, canonicalHost(canonical)); value != "" {
			related = append(related, value)
		}
	}
	github := &GitHubWork{Kind: "pull_request", Number: raw.Number, URL: canonical, State: raw.State, ReviewDecision: raw.ReviewDecision, RelatedIssueURLs: uniqueStrings(related), RelatedPullRequestURLs: []string{}, Checks: mapChecks(raw.StatusCheckRollup), Fields: map[string]string{}, ContentAvailable: true, ObservedAt: timePtr(observed), Stale: stale}
	return PullRequestDetail{URL: canonical, GitHub: github}
}

func cachedPullRequestDetail(value any, canonical string, stale bool) PullRequestDetail {
	github, _ := value.(*GitHubWork)
	if github == nil {
		return PullRequestDetail{URL: canonical}
	}
	copy := *github
	copy.URL = canonical
	copy.RelatedIssueURLs = append([]string(nil), github.RelatedIssueURLs...)
	copy.RelatedPullRequestURLs = append([]string(nil), github.RelatedPullRequestURLs...)
	copy.Checks = append([]GitHubCheck(nil), github.Checks...)
	copy.Fields = cloneStringMap(github.Fields)
	copy.Stale = stale
	return PullRequestDetail{URL: canonical, GitHub: &copy}
}

func canonicalHost(canonical string) string {
	parsed, _ := url.Parse(canonical)
	return strings.ToLower(parsed.Host)
}

func canonicalIssueURL(raw, host string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || strings.ToLower(parsed.Host) != strings.ToLower(host) || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "issues" || !repoPattern.MatchString(parts[0]+"/"+parts[1]) {
		return ""
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/%s/%s/issues/%d", host, parts[0], parts[1], number)
}

func locatorIdentity(raw, host string) string {
	canonical := canonicalItemURL(raw, host)
	if canonical == "" {
		return ""
	}
	parsed, err := url.Parse(canonical)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 {
		return ""
	}
	return strings.ToLower(parsed.Host + "/" + parts[0] + "/" + parts[1] + "/" + parts[2] + "/" + parts[3])
}

func pullRequestDetailNotice(err error) string {
	if err == nil {
		return ""
	}
	if diagnostic := safeGitHubCommandDiagnostic(err.Error()); diagnostic != "" {
		return "Pull request 조회 실패: " + diagnostic
	}
	return "Pull request 조회에 실패했습니다. GitHub 연결과 권한을 확인하세요."
}
