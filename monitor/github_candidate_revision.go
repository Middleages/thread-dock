package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// enrichAgentCandidateRevisions resolves the immutable head revision for PRs
// that are already correlated with an observed AgentWorkflow. GitHub's PR list
// payload currently does not expose headRefOid in the canonical read model, so
// this is a narrow follow-up read rather than a second source of work state.
// Failure to resolve a revision is non-fatal: verification remains unverified.
func enrichAgentCandidateRevisions(snapshot *Snapshot, process CommandRunner, distribution string, timeoutDuration int64) []string {
	if snapshot == nil || process == nil {
		return nil
	}
	runner := newWSLCommandRunner(process, distribution, durationFromNanos(timeoutDuration))
	if runner == nil {
		return nil
	}
	notices := []string{}
	for projectIndex := range snapshot.Projects {
		project := &snapshot.Projects[projectIndex]
		for workIndex := range project.WorkItems {
			work := &project.WorkItems[workIndex]
			if work.AgentWorkflow == nil || work.GitHub == nil || work.GitHub.Kind != "pull_request" || work.GitHub.Number == nil {
				continue
			}
			if work.GitHub.Fields == nil {
				work.GitHub.Fields = map[string]string{}
			}
			if strings.TrimSpace(firstNonEmpty(work.GitHub.Fields["headSha"], work.GitHub.Fields["sha"])) != "" {
				continue
			}
			sha, err := fetchPullRequestHeadSHA(runner, *work.GitHub.Number, work.GitHub.URL)
			if err != nil {
				notices = append(notices, fmt.Sprintf("PR #%d의 head SHA를 확인하지 못했습니다.", *work.GitHub.Number))
				continue
			}
			work.GitHub.Fields["headSha"] = sha
		}
	}
	return uniqueStrings(notices)
}

func durationFromNanos(value int64) interface{ } {
	return durationValue(value)
}

func durationValue(value int64) timeDuration {
	return timeDuration(value)
}

type timeDuration int64

func (d timeDuration) toDuration() interface{} { return d }

func fetchPullRequestHeadSHA(runner *wslCommandRunner, number int, prURL string) (string, error) {
	if runner == nil {
		return "", errors.New("github command runner is unavailable")
	}
	args := []string{"pr", "view"}
	if strings.TrimSpace(prURL) != "" {
		args = append(args, prURL)
	} else {
		args = append(args, strconv.Itoa(number))
	}
	args = append(args, "--json", "headRefOid", "--jq", ".headRefOid")
	output, err := runner.run(context.Background(), "gh", args...)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(output))
	if sha == "" {
		return "", errors.New("empty pull request head SHA")
	}
	return sha, nil
}
